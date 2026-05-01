// TaskFlow Windows desktop entry point.
//
// Boot order:
//   1. Initialize logging.
//   2. Open local SQLite cache (created on first run).
//   3. Load AppConfig from disk (server URL, tokens, autostart preference).
//   4. Spawn background tasks:
//       - Sync loop: pulls /api/sync/pull every 30 s when token present.
//       - Scheduler loop: every 5 s scans local reminder cache, fires
//         strong-reminders that hit their next_fire_at.
//   5. Build Tauri tray + main window + register #[tauri::command] handlers.
//
// Strong-reminder pipeline (offline-safe by design — see spec §4 / §7):
//   scheduler::tick() -> alarm::fire(rule):
//       1. Show Windows toast via tauri-plugin-notification.
//       2. Open / focus a dedicated full-screen alarm window
//          (label "alarm-<rule_id>"), always-on-top.
//       3. Start ringing on a background rodio Sink.
//       4. Window publishes a "stop" event; main process kills sink and
//          closes window. If user clicked "complete", we call the API only
//          if online — offline only stops the local ring (spec §4).

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod alarm;
mod api;
mod commands;
mod config;
mod db;
mod notify;
mod scheduler;
mod sync;
mod tray;

use std::sync::Arc;
use tauri::{Manager, RunEvent};
use tokio::sync::Mutex;

pub struct AppState {
    pub cfg: Mutex<config::AppConfig>,
    pub db: Arc<db::LocalDb>,
    pub api: api::ApiClient,
    pub alarm: alarm::AlarmController,
}

fn main() {
    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("info")).init();

    let app_dir = config::default_app_dir().expect("resolve app dir");
    std::fs::create_dir_all(&app_dir).expect("create app dir");
    let cfg = config::AppConfig::load_or_default(&app_dir);
    let db = Arc::new(db::LocalDb::open(&app_dir.join("cache.db")).expect("open local db"));
    let api = api::ApiClient::new(cfg.server_url.clone(), cfg.access_token.clone());
    let alarm = alarm::AlarmController::new();

    let state = AppState {
        cfg: Mutex::new(cfg),
        db: db.clone(),
        api,
        alarm: alarm.clone(),
    };

    let app = tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_os::init())
        .plugin(tauri_plugin_process::init())
        .plugin(
            tauri_plugin_autostart::init(
                tauri_plugin_autostart::MacosLauncher::LaunchAgent,
                Some(vec!["--minimized"]),
            ),
        )
        .manage(state)
        .invoke_handler(tauri::generate_handler![
            commands::set_server_config,
            commands::get_server_config,
            commands::set_tokens,
            commands::clear_tokens,
            commands::set_autostart,
            commands::is_autostart_enabled,
            commands::sync_now,
            commands::stop_alarm,
            commands::open_external,
            commands::quit_app,
        ])
        .setup(move |app| {
            // Tray icon + menu
            tray::setup(app)?;

            // Background tasks
            let handle = app.handle().clone();
            let db_for_sync = db.clone();
            let db_for_sched = db.clone();

            // Sync loop
            tauri::async_runtime::spawn(async move {
                sync::run_sync_loop(handle, db_for_sync).await;
            });

            // Scheduler loop
            let handle2 = app.handle().clone();
            let alarm2 = alarm.clone();
            tauri::async_runtime::spawn(async move {
                scheduler::run_scheduler_loop(handle2, db_for_sched, alarm2).await;
            });

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application");

    // Hide window when user clicks the X — keep app running in tray.
    // User can fully quit via tray "退出" or commands::quit_app.
    app.run(|app_handle, event| match event {
        RunEvent::ExitRequested { api, .. } => {
            // 默认行为:不退出,藏起来
            if let Some(win) = app_handle.get_webview_window("main") {
                let _ = win.hide();
            }
            api.prevent_exit();
        }
        _ => {}
    });
}
