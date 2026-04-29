// Local scheduler loop. Wakes every 5 s, asks the local cache for any
// rule whose `next_fire_at <= now` that hasn't already been logged,
// and fires the alarm pipeline for each.
//
// Spec §4 (offline) requires this to keep working when the network is
// down. Because we never delete a rule from the cache except on explicit
// "deleted" sync events, the scheduler is fully autonomous.

use crate::alarm::AlarmController;
use crate::db::LocalDb;
use chrono::Utc;
use std::sync::Arc;
use std::time::Duration;
use tauri::AppHandle;

pub async fn run_scheduler_loop(handle: AppHandle, db: Arc<LocalDb>, alarm: AlarmController) {
    // Small initial delay so the UI mounts first.
    tokio::time::sleep(Duration::from_secs(2)).await;
    let mut interval = tokio::time::interval(Duration::from_secs(5));
    loop {
        interval.tick().await;
        if let Err(e) = tick(&handle, &db, &alarm).await {
            log::warn!("scheduler tick error: {:#}", e);
        }
    }
}

async fn tick(handle: &AppHandle, db: &LocalDb, alarm: &AlarmController) -> anyhow::Result<()> {
    let now = Utc::now();
    let due = db.list_due(now, 16)?;
    if due.is_empty() {
        return Ok(());
    }
    log::info!("scheduler: firing {} reminder(s)", due.len());
    for rule in due {
        let fire_at = rule.next_fire_at.unwrap_or(now);
        // Mark fired BEFORE showing UI so a panic in alarm code can't loop.
        db.log_fire(rule.id, fire_at)?;
        alarm.fire(handle, rule, fire_at).await;
    }
    Ok(())
}
