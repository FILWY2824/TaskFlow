// IPC commands the Vue frontend can invoke via @tauri-apps/api `invoke()`.
//
// Naming convention: snake_case Rust ↔ snake_case TS,
// e.g. invoke('set_tokens', { access, refresh, userId }).
//
// Notable: most "user-facing" commands here are bridges so the webview can
// hand the Rust side authoritative bits (access token, server URL,
// autostart preference). The webview remains the source of truth for
// user-facing CRUD; Rust only owns offline-safe scheduling.

use crate::config::AppConfig;
use crate::AppState;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Manager, State};
use tauri_plugin_autostart::ManagerExt;

#[derive(Debug, Serialize)]
pub struct ServerConfigOut {
    pub server_url: String,
    pub timezone: String,
    pub autostart: bool,
}

#[tauri::command]
pub async fn get_server_config(state: State<'_, AppState>) -> Result<ServerConfigOut, String> {
    let cfg = state.cfg.lock().await;
    Ok(ServerConfigOut {
        server_url: cfg.server_url.clone(),
        timezone: cfg.timezone.clone(),
        autostart: cfg.autostart,
    })
}

#[derive(Debug, Deserialize)]
pub struct SetServerConfigArgs {
    pub server_url: String,
    pub timezone: Option<String>,
}

#[tauri::command]
pub async fn set_server_config(
    args: SetServerConfigArgs,
    state: State<'_, AppState>,
) -> Result<(), String> {
    let mut cfg = state.cfg.lock().await;
    cfg.server_url = args.server_url.clone();
    if let Some(tz) = args.timezone {
        cfg.timezone = tz;
    }
    state.api.set_base(cfg.server_url.clone());
    let app_dir = crate::config::default_app_dir().map_err(|e| e.to_string())?;
    cfg.save(&app_dir).map_err(|e| e.to_string())?;
    Ok(())
}

#[derive(Debug, Deserialize)]
pub struct SetTokensArgs {
    pub access_token: Option<String>,
    pub refresh_token: Option<String>,
    pub timezone: Option<String>,
}

/// Called by the Vue side after login / refresh so the Rust process
/// has the same access token in hand for background sync.
#[tauri::command]
pub async fn set_tokens(args: SetTokensArgs, state: State<'_, AppState>) -> Result<(), String> {
    let mut cfg = state.cfg.lock().await;
    cfg.access_token = args.access_token.clone();
    if args.refresh_token.is_some() {
        cfg.refresh_token = args.refresh_token;
    }
    if let Some(tz) = args.timezone {
        cfg.timezone = tz;
    }
    state.api.set_token(cfg.access_token.clone());
    let app_dir = crate::config::default_app_dir().map_err(|e| e.to_string())?;
    cfg.save(&app_dir).map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn clear_tokens(state: State<'_, AppState>) -> Result<(), String> {
    let mut cfg = state.cfg.lock().await;
    cfg.access_token = None;
    cfg.refresh_token = None;
    state.api.set_token(None);
    let app_dir = crate::config::default_app_dir().map_err(|e| e.to_string())?;
    cfg.save(&app_dir).map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn set_autostart(enabled: bool, app: AppHandle, state: State<'_, AppState>) -> Result<(), String> {
    let manager = app.autolaunch();
    if enabled {
        manager.enable().map_err(|e| e.to_string())?;
    } else {
        manager.disable().map_err(|e| e.to_string())?;
    }
    let mut cfg = state.cfg.lock().await;
    cfg.autostart = enabled;
    let app_dir = crate::config::default_app_dir().map_err(|e| e.to_string())?;
    cfg.save(&app_dir).map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub fn is_autostart_enabled(app: AppHandle) -> Result<bool, String> {
    app.autolaunch().is_enabled().map_err(|e| e.to_string())
}

/// Kick the sync loop right now.
#[tauri::command]
pub async fn sync_now(state: State<'_, AppState>) -> Result<(), String> {
    crate::sync::run_once(&state.api, &state.db)
        .await
        .map_err(|e| format!("{:#}", e))
}

/// Stop a currently-ringing alarm. Called from the alarm window's
/// "停止" / "完成" / "稍后" buttons.
#[tauri::command]
pub async fn stop_alarm(rule_id: i64, app: AppHandle, state: State<'_, AppState>) -> Result<(), String> {
    state.alarm.stop(&app, rule_id);
    Ok(())
}

/// Open a URL in the user's default browser. Used for Telegram deep links
/// (https://t.me/...) and for opening the bound server's web UI.
#[tauri::command]
pub fn open_external(url: String) -> Result<(), String> {
    // Tauri 2's plugin-shell isn't enabled here to keep the binary small;
    // a manual `Command::new` covers our two use cases (Telegram + admin URL).
    #[cfg(windows)]
    {
        std::process::Command::new("cmd")
            .args(["/C", "start", "", &url])
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .arg(&url)
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(all(unix, not(target_os = "macos")))]
    {
        std::process::Command::new("xdg-open")
            .arg(&url)
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    Ok(())
}

/// Force-quit (bypass tray-hide). Tray menu's "退出" calls this.
#[tauri::command]
pub fn quit_app(app: AppHandle) {
    app.exit(0);
}

// 同步配置到 AppConfig 也在 Vue side 触发,用 `_state` 避免 unused 警告
#[allow(dead_code)]
fn _touch(_state: &AppConfig) {}
