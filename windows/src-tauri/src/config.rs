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
    "UTC".to_string()
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

/// 返回 <exe所在目录>/data 作为数据目录(便携化,所有数据跟着程序走)。
/// Windows 上 exe 放在 Program Files 时需要写权限;安装时给 data/ 目录设好 ACL
/// 或者安装时不选 Program Files,让用户选有写权限的目录(如 %LOCALAPPDATA%\Programs)。
pub fn default_app_dir() -> Result<PathBuf> {
    let exe = std::env::current_exe().context("current_exe")?;
    let exe_dir = exe.parent().context("exe parent dir")?;
    Ok(exe_dir.join("data"))
}
