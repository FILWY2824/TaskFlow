// On-disk app configuration: server URL, auth tokens, user preferences.
//
// File layout (JSON, single file to keep lock-free read/write simple):
//   %APPDATA%/TodoAlarm/config.json
//   %APPDATA%/TodoAlarm/cache.db          (SQLite, in db.rs)
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
    // 默认指向开发后端;首次启动后用户在前端"设置"里填实际服务端
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

/// %APPDATA%/TodoAlarm on Windows, $XDG_CONFIG_HOME/todoalarm elsewhere.
pub fn default_app_dir() -> Result<PathBuf> {
    #[cfg(windows)]
    {
        let base = std::env::var_os("APPDATA")
            .map(PathBuf::from)
            .context("APPDATA not set")?;
        Ok(base.join("TodoAlarm"))
    }
    #[cfg(not(windows))]
    {
        let base = std::env::var_os("XDG_CONFIG_HOME")
            .map(PathBuf::from)
            .or_else(|| std::env::var_os("HOME").map(|h| PathBuf::from(h).join(".config")))
            .context("HOME not set")?;
        Ok(base.join("todoalarm"))
    }
}
