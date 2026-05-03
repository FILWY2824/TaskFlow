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
pub async fn set_autostart(
    enabled: bool,
    app: AppHandle,
    state: State<'_, AppState>,
) -> Result<(), String> {
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
pub async fn stop_alarm(
    rule_id: i64,
    app: AppHandle,
    state: State<'_, AppState>,
) -> Result<(), String> {
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
        use windows::core::PCWSTR;
        use windows::Win32::UI::Shell::ShellExecuteW;
        use windows::Win32::UI::WindowsAndMessaging::SW_SHOW;
        // 用 ShellExecuteW 而非 cmd /c start,避免 cmd.exe 把 URL 中 & 解释为命令分隔符。
        let url_w: Vec<u16> = url.encode_utf16().chain(std::iter::once(0)).collect();
        let op_w: Vec<u16> = "open".encode_utf16().chain(std::iter::once(0)).collect();
        unsafe {
            ShellExecuteW(
                None,
                PCWSTR(op_w.as_ptr()),
                PCWSTR(url_w.as_ptr()),
                None,
                None,
                SW_SHOW,
            );
        }
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
    crate::QUIT_FLAG.store(true, std::sync::atomic::Ordering::SeqCst);
    app.exit(0);
}

/// 给前端用:返回打包时烧进去的"默认服务端 URL"(env VITE_TASKFLOW_DEFAULT_SERVER
/// 或 TASKFLOW_DEFAULT_SERVER_URL),前端在第一次启动时用这个值做 server_url
/// 的初始值。允许空 —— 空时前端走"必须先到设置页填地址"的引导。
#[tauri::command]
pub fn get_default_server_url() -> String {
    // 编译时常量(由 build.rs / Cargo 注入)。运行时再叠一次环境变量,
    // 让用户在 Windows 上启动前用 set TASKFLOW_DEFAULT_SERVER_URL=... 临时覆盖。
    if let Ok(v) = std::env::var("TASKFLOW_DEFAULT_SERVER_URL") {
        if !v.trim().is_empty() {
            return v.trim().trim_end_matches('/').to_string();
        }
    }
    let baked = option_env!("TASKFLOW_DEFAULT_SERVER_URL").unwrap_or("");
    baked.trim().trim_end_matches('/').to_string()
}

/// 把主窗口拉到屏幕最前 + 抢焦点。供前端在收到强提醒、会话过期等需要
/// "立刻看到的事情" 时调用。强提醒本身已经会开独立 alarm 窗口,这个
/// 命令是给主窗口准备的(例如 SSE 推到一条新通知)。
#[tauri::command]
pub fn bring_window_to_front(label: Option<String>, app: AppHandle) -> Result<(), String> {
    let lbl = label.unwrap_or_else(|| "main".to_string());
    let win = app
        .get_webview_window(&lbl)
        .ok_or_else(|| format!("window '{}' not found", lbl))?;
    crate::tray::bring_window_to_front(&win);
    Ok(())
}

// 同步配置到 AppConfig 也在 Vue side 触发,用 `_state` 避免 unused 警告
#[allow(dead_code)]
fn _touch(_state: &AppConfig) {}
