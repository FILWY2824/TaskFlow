// Local SQLite cache. Mirrors *only* the data the Tauri Rust process needs
// to fire offline-safe alarms: reminder rules + last-known sync cursor.
//
// We deliberately do NOT mirror todos / lists / subtasks / notifications
// here — those live in localStorage on the Vue side and don't need offline
// scheduling.
//
// Schema:
//   reminder_rules_cache(
//     id, user_id, todo_id, title, next_fire_at, rrule,
//     channel_local, ringtone, fullscreen, vibrate, is_enabled,
//     synced_at
//   )
//   sync_cursor(user_id PRIMARY KEY, cursor INTEGER)
//   local_alarm_log(rule_id, fire_at) — to dedupe when the scheduler tick
//     comes around twice for the same fire instance.

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use rusqlite::{params, Connection, OptionalExtension};
use std::path::Path;
use std::sync::Mutex;

pub struct LocalDb {
    conn: Mutex<Connection>,
}

#[derive(Debug, Clone)]
pub struct CachedRule {
    pub id: i64,
    pub user_id: i64,
    pub todo_id: Option<i64>,
    pub title: String,
    pub next_fire_at: Option<DateTime<Utc>>,
    pub channel_local: bool,
    pub ringtone: String,
    pub fullscreen: bool,
    pub vibrate: bool,
    pub is_enabled: bool,
}

impl LocalDb {
    pub fn open(path: &Path) -> Result<Self> {
        let conn = Connection::open(path).context("open local sqlite cache")?;
        conn.execute_batch(
            r#"
            PRAGMA journal_mode = WAL;
            PRAGMA synchronous = NORMAL;
            PRAGMA foreign_keys = ON;

            CREATE TABLE IF NOT EXISTS reminder_rules_cache (
                id INTEGER PRIMARY KEY,
                user_id INTEGER NOT NULL,
                todo_id INTEGER,
                title TEXT NOT NULL DEFAULT '',
                next_fire_at TEXT,           -- RFC3339 UTC, NULL when disabled / no upcoming
                rrule TEXT NOT NULL DEFAULT '',
                channel_local INTEGER NOT NULL DEFAULT 1,
                ringtone TEXT NOT NULL DEFAULT 'default',
                fullscreen INTEGER NOT NULL DEFAULT 1,
                vibrate INTEGER NOT NULL DEFAULT 1,
                is_enabled INTEGER NOT NULL DEFAULT 1,
                synced_at TEXT NOT NULL
            );
            CREATE INDEX IF NOT EXISTS idx_cache_next ON reminder_rules_cache(next_fire_at)
                WHERE is_enabled = 1 AND channel_local = 1 AND next_fire_at IS NOT NULL;

            CREATE TABLE IF NOT EXISTS sync_cursor (
                user_id INTEGER PRIMARY KEY,
                cursor INTEGER NOT NULL DEFAULT 0,
                updated_at TEXT NOT NULL
            );

            CREATE TABLE IF NOT EXISTS local_alarm_log (
                rule_id INTEGER NOT NULL,
                fire_at TEXT NOT NULL,         -- RFC3339 UTC, the instance that fired
                fired_at TEXT NOT NULL,        -- when we actually showed the alarm
                acked_at TEXT,                 -- when user clicked stop / complete locally
                PRIMARY KEY(rule_id, fire_at)
            );
            "#,
        )
        .context("init schema")?;
        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    pub fn get_cursor(&self, user_id: i64) -> Result<i64> {
        let conn = self.conn.lock().unwrap();
        let cur: Option<i64> = conn
            .query_row(
                "SELECT cursor FROM sync_cursor WHERE user_id = ?1",
                params![user_id],
                |r| r.get(0),
            )
            .optional()?;
        Ok(cur.unwrap_or(0))
    }

    pub fn set_cursor(&self, user_id: i64, cursor: i64) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO sync_cursor(user_id, cursor, updated_at)
             VALUES (?1, ?2, ?3)
             ON CONFLICT(user_id) DO UPDATE SET cursor = excluded.cursor, updated_at = excluded.updated_at",
            params![user_id, cursor, Utc::now().to_rfc3339()],
        )?;
        Ok(())
    }

    /// Upsert one rule from a server pull. Pass None for `next_fire_at` to
    /// effectively disable scheduling without deleting history.
    pub fn upsert_rule(&self, rule: &CachedRule) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO reminder_rules_cache(
                id, user_id, todo_id, title, next_fire_at, rrule,
                channel_local, ringtone, fullscreen, vibrate, is_enabled, synced_at
             )
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
             ON CONFLICT(id) DO UPDATE SET
               user_id = excluded.user_id,
               todo_id = excluded.todo_id,
               title = excluded.title,
               next_fire_at = excluded.next_fire_at,
               rrule = excluded.rrule,
               channel_local = excluded.channel_local,
               ringtone = excluded.ringtone,
               fullscreen = excluded.fullscreen,
               vibrate = excluded.vibrate,
               is_enabled = excluded.is_enabled,
               synced_at = excluded.synced_at",
            params![
                rule.id,
                rule.user_id,
                rule.todo_id,
                rule.title,
                rule.next_fire_at.map(|t| t.to_rfc3339()),
                "",
                rule.channel_local as i64,
                rule.ringtone,
                rule.fullscreen as i64,
                rule.vibrate as i64,
                rule.is_enabled as i64,
                Utc::now().to_rfc3339(),
            ],
        )?;
        Ok(())
    }

    pub fn delete_rule(&self, id: i64) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "DELETE FROM reminder_rules_cache WHERE id = ?1",
            params![id],
        )?;
        Ok(())
    }

    /// Rules that should fire now (next_fire_at <= now) and have not been
    /// logged yet. Returns up to `limit` rules.
    pub fn list_due(&self, now: DateTime<Utc>, limit: i64) -> Result<Vec<CachedRule>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn.prepare(
            "SELECT r.id, r.user_id, r.todo_id, r.title, r.next_fire_at,
                    r.channel_local, r.ringtone, r.fullscreen, r.vibrate, r.is_enabled
             FROM reminder_rules_cache r
             WHERE r.is_enabled = 1
               AND r.channel_local = 1
               AND r.next_fire_at IS NOT NULL
               AND r.next_fire_at <= ?1
               AND NOT EXISTS (
                   SELECT 1 FROM local_alarm_log l
                   WHERE l.rule_id = r.id AND l.fire_at = r.next_fire_at
               )
             ORDER BY r.next_fire_at ASC
             LIMIT ?2",
        )?;
        let rows = stmt.query_map(params![now.to_rfc3339(), limit], |row| {
            let next: Option<String> = row.get(4)?;
            Ok(CachedRule {
                id: row.get(0)?,
                user_id: row.get(1)?,
                todo_id: row.get(2)?,
                title: row.get(3)?,
                next_fire_at: next.and_then(|s| {
                    DateTime::parse_from_rfc3339(&s)
                        .ok()
                        .map(|t| t.with_timezone(&Utc))
                }),
                channel_local: row.get::<_, i64>(5)? != 0,
                ringtone: row.get(6)?,
                fullscreen: row.get::<_, i64>(7)? != 0,
                vibrate: row.get::<_, i64>(8)? != 0,
                is_enabled: row.get::<_, i64>(9)? != 0,
            })
        })?;
        let mut out = Vec::new();
        for r in rows {
            out.push(r?);
        }
        Ok(out)
    }

    /// Record that we showed an alarm for (rule_id, fire_at). Idempotent.
    pub fn log_fire(&self, rule_id: i64, fire_at: DateTime<Utc>) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT OR IGNORE INTO local_alarm_log(rule_id, fire_at, fired_at)
             VALUES (?1, ?2, ?3)",
            params![rule_id, fire_at.to_rfc3339(), Utc::now().to_rfc3339()],
        )?;
        Ok(())
    }

    pub fn mark_acked(&self, rule_id: i64, fire_at: DateTime<Utc>) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "UPDATE local_alarm_log SET acked_at = ?1
             WHERE rule_id = ?2 AND fire_at = ?3",
            params![Utc::now().to_rfc3339(), rule_id, fire_at.to_rfc3339()],
        )?;
        Ok(())
    }
}
