// Background sync loop. Runs every 30 s when an access token is present.
//
// On each tick:
//   1. Pull /api/auth/me (also doubles as a token-validity probe; silently
//      bails out on 401).
//   2. Drain /api/sync/pull from the cached cursor. For each event:
//       - reminder created/updated -> GET /api/reminders/{id} and upsert
//         into local cache (so scheduler can fire it offline).
//       - reminder deleted -> delete from cache.
//       - other entity types -> ignore (webview handles UI refresh via
//         its own SSE subscription).
//   3. Push the latest cursor back to local cache.
//
// Kicked manually by `commands::sync_now` and `tray` on demand.

use crate::api::ApiClient;
use crate::db::{CachedRule, LocalDb};
use crate::AppState;
use std::sync::Arc;
use std::time::Duration;
use tauri::{AppHandle, Manager};

pub async fn run_sync_loop(handle: AppHandle, db: Arc<LocalDb>) {
    // Wait a couple of seconds so the webview can hand us a token first.
    tokio::time::sleep(Duration::from_secs(3)).await;
    let mut interval = tokio::time::interval(Duration::from_secs(30));
    loop {
        interval.tick().await;
        let state = handle.state::<AppState>();
        if !state.api.token_present() {
            log::debug!("sync: no token yet, skipping");
            continue;
        }
        if let Err(e) = run_once(&state.api, &db).await {
            log::warn!("sync error: {:#}", e);
        }
    }
}

pub async fn run_once(api: &ApiClient, db: &LocalDb) -> anyhow::Result<()> {
    let me = api.me().await?;
    let cursor = db.get_cursor(me.id).unwrap_or(0);

    let mut current = cursor;
    let mut iterations = 0;
    loop {
        let page = api.pull(current, 200).await?;
        for ev in &page.events {
            match (ev.entity_type.as_str(), ev.action.as_str()) {
                ("reminder", "deleted") => {
                    let _ = db.delete_rule(ev.entity_id);
                }
                ("reminder", _) => {
                    if let Ok(rule) = api.get_reminder(ev.entity_id).await {
                        let cached = CachedRule {
                            id: rule.id,
                            user_id: rule.user_id,
                            todo_id: rule.todo_id,
                            title: rule.title,
                            next_fire_at: if rule.is_enabled {
                                rule.next_fire_at
                            } else {
                                None
                            },
                            channel_local: rule.channel_local,
                            ringtone: rule.ringtone,
                            fullscreen: rule.fullscreen,
                            vibrate: rule.vibrate,
                            is_enabled: rule.is_enabled,
                        };
                        let _ = db.upsert_rule(&cached);
                    }
                }
                _ => {} // 其他实体类型,Webview 自己刷新即可
            }
        }
        current = page.next_cursor;
        if !page.has_more {
            break;
        }
        iterations += 1;
        if iterations > 50 {
            log::warn!("sync: too many pages in one round, deferring rest to next tick");
            break;
        }
    }
    if current > cursor {
        let _ = db.set_cursor(me.id, current);
    }
    Ok(())
}

/// First-time bootstrap (or after re-login): fetch the full reminder list
/// up front so we don't have to wait for sync_events to gradually surface
/// pre-existing rules.
pub async fn full_resync(api: &ApiClient, db: &LocalDb) -> anyhow::Result<()> {
    let me = api.me().await?;
    let rules = api.list_reminders().await?;
    for rule in rules {
        let cached = CachedRule {
            id: rule.id,
            user_id: rule.user_id,
            todo_id: rule.todo_id,
            title: rule.title,
            next_fire_at: if rule.is_enabled { rule.next_fire_at } else { None },
            channel_local: rule.channel_local,
            ringtone: rule.ringtone,
            fullscreen: rule.fullscreen,
            vibrate: rule.vibrate,
            is_enabled: rule.is_enabled,
        };
        let _ = db.upsert_rule(&cached);
    }
    // 把 cursor 推到当前最新,后续靠增量同步追
    if let Ok(page) = api.pull(0, 1).await {
        if page.next_cursor > 0 {
            let _ = db.set_cursor(me.id, page.next_cursor);
        }
    }
    Ok(())
}
