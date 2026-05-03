// On-disk app configuration: server URL, auth tokens, user preferences.
//
// File layout (all data co-located with the executable, NOT in AppData):
//   <exe_dir>/data/config.json
//   <exe_dir>/data/cache.db          (SQLite, in db.rs)
//
// We deliberately do NOT store any business state here. Cached todos /
// reminders live in cache.db; UI session state lives in the webview's
// localStorage (managed by the Vue app).

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AppConfig {
    /// Server base URL, e.g. "https://todo.example.com" or "http://127.0.0.1:8080"
    #[serde(default = "default_server_url")]
    pub server_url: String,

    /// JWT access token (refresh handled by webview / Vue side; we mirror it
    /// here so background Rust tasks can also call the API).
    #[serde(default)]
    pub access_token: Option<String>,

    /// Refresh token, used by background sync if access expires.
    #[serde(default)]
    pub refresh_token: Option<String>,

    /// User timezone (IANA), used for stats & local day boundaries.
    /// Mirrors what the Vue side already learned from /api/auth/me.
    #[serde(default = "default_tz")]
    pub timezone: String,

    /// Whether the app should run on Windows login.
    #[serde(default)]
    pub autostart: bool,

    /// Local ringtone choice. "default" = built-in beep loop.
    /// (extensible: in the future we can let the user point at a custom .wav)
    #[serde(default = "default_ringtone")]
    pub ringtone: String,
}

fn default_server_url() -> String {
    // 优先级:
    //   1) 运行时环境变量 TASKFLOW_DEFAULT_SERVER_URL(临时覆盖)
    //   2) 编译时 env TASKFLOW_DEFAULT_SERVER_URL(打包时由 .env 注入)
    //   3) 兜底: http://127.0.0.1:8080(本地开发)
    if let Ok(v) = std::env::var("TASKFLOW_DEFAULT_SERVER_URL") {
        let v = v.trim().trim_end_matches('/');
        if !v.is_empty() {
            return v.to_string();
        }
    }
    let baked = option_env!("TASKFLOW_DEFAULT_SERVER_URL").unwrap_or("").trim();
    if !baked.is_empty() {
        return baked.trim_end_matches('/').to_string();
    }
    "http://127.0.0.1:8080".to_string()
}

fn default_tz() -> String {
    "Asia/Shanghai".to_string()
}

fn default_ringtone() -> String {
    "default".to_string()
}

impl Default for AppConfig {
    fn default() -> Self {
        Self {
            server_url: default_server_url(),
            access_token: None,
            refresh_token: None,
            timezone: default_tz(),
            autostart: false,
            ringtone: default_ringtone(),
        }
    }
}

impl AppConfig {
    pub fn load_or_default(app_dir: &Path) -> Self {
        let path = app_dir.join("config.json");
        if path.exists() {
            match std::fs::read_to_string(&path)
                .ok()
                .and_then(|s| serde_json::from_str::<AppConfig>(&s).ok())
            {
                Some(cfg) => return cfg,
                None => log::warn!("config.json corrupted, falling back to defaults"),
            }
        }
        Self::default()
    }

    pub fn save(&self, app_dir: &Path) -> Result<()> {
        let path = app_dir.join("config.json");
        let s = serde_json::to_string_pretty(self).context("serialize config")?;
        // 原子写:写到 .tmp 再 rename
        let tmp = app_dir.join("config.json.tmp");
        std::fs::write(&tmp, s).context("write config.json.tmp")?;
        std::fs::rename(&tmp, &path).context("rename config.json")?;
        Ok(())
    }
}

/// 数据目录:优先 exe 目录(便携),不可写时回退到 %LOCALAPPDATA%/TaskFlow。
pub fn default_app_dir() -> Result<PathBuf> {
    let exe = std::env::current_exe().context("current_exe")?;
    let exe_dir = exe.parent().context("exe parent dir")?;
    let portable = exe_dir.join("data");

    // 如果 data 目录已存在且可写,或者可以创建,就用便携路径
    if let Ok(()) = ensure_writable(&portable) {
        return Ok(portable);
    }

    // 否则回退 AppData(MSI 装到 Program Files 的情况)
    #[cfg(windows)]
    {
        let appdata = std::env::var_os("LOCALAPPDATA")
            .map(PathBuf::from)
            .context("LOCALAPPDATA not set")?;
        let d = appdata.join("TaskFlow").join("data");
        std::fs::create_dir_all(&d).ok();
        return Ok(d);
    }
    #[cfg(not(windows))]
    {
        let home = std::env::var_os("HOME")
            .map(PathBuf::from)
            .context("HOME not set")?;
        let d = home.join(".local").join("share").join("taskflow");
        std::fs::create_dir_all(&d).ok();
        return Ok(d);
    }
}

fn ensure_writable(dir: &PathBuf) -> Result<(), ()> {
    std::fs::create_dir_all(dir).map_err(|_| ())?;
    let test = dir.join(".rw_test");
    std::fs::write(&test, b"x").map_err(|_| ())?;
    std::fs::remove_file(&test).map_err(|_| ())?;
    Ok(())
}
