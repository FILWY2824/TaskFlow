// Minimal HTTP client used by the background Rust process (sync loop).
// The webview does its own auth + API calls in TS via fetch(). This client
// exists because:
//   * Background sync needs to run even when no webview is open.
//   * Background scheduler needs to (eventually, when online) post local
//     completion / "snooze" actions.
//
// We do NOT mirror the full server API here — only the endpoints the
// background tasks need. The webview remains the source of truth for
// user-facing CRUD.

use anyhow::{anyhow, Context, Result};
use chrono::{DateTime, Utc};
use reqwest::{Client, StatusCode};
use serde::{Deserialize, Serialize};
use std::sync::{Arc, RwLock};
use std::time::Duration;

#[derive(Clone)]
pub struct ApiClient {
    inner: Arc<Inner>,
}

struct Inner {
    base: RwLock<String>,
    token: RwLock<Option<String>>,
    http: Client,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SyncEvent {
    pub id: i64,
    pub entity_type: String,
    pub entity_id: i64,
    pub action: String,
    pub created_at: String,
}

#[derive(Debug, Deserialize)]
pub struct SyncPullResponse {
    pub events: Vec<SyncEvent>,
    pub next_cursor: i64,
    pub has_more: bool,
}

#[derive(Debug, Deserialize)]
pub struct ReminderRule {
    pub id: i64,
    pub user_id: i64,
    pub todo_id: Option<i64>,
    pub title: String,
    pub trigger_at: Option<DateTime<Utc>>,
    pub rrule: String,
    pub timezone: String,
    pub channel_local: bool,
    pub channel_telegram: bool,
    pub channel_web_push: bool,
    pub is_enabled: bool,
    pub next_fire_at: Option<DateTime<Utc>>,
    pub ringtone: String,
    pub vibrate: bool,
    pub fullscreen: bool,
}

#[derive(Debug, Deserialize)]
pub struct RemindersListResponse {
    pub items: Vec<ReminderRule>,
}

#[derive(Debug, Deserialize)]
pub struct UserMe {
    pub id: i64,
    pub email: String,
    pub display_name: String,
    pub timezone: String,
}

impl ApiClient {
    pub fn new(base: String, token: Option<String>) -> Self {
        let http = Client::builder()
            .user_agent("taskflow-windows/1.4.0")
            .timeout(Duration::from_secs(20))
            .build()
            .expect("build reqwest client");
        Self {
            inner: Arc::new(Inner {
                base: RwLock::new(base),
                token: RwLock::new(token),
                http,
            }),
        }
    }

    pub fn set_base(&self, base: String) {
        *self.inner.base.write().unwrap() = base;
    }

    pub fn set_token(&self, token: Option<String>) {
        *self.inner.token.write().unwrap() = token;
    }

    pub fn token_present(&self) -> bool {
        self.inner.token.read().unwrap().is_some()
    }

    fn url(&self, path: &str) -> String {
        let base = self.inner.base.read().unwrap();
        format!("{}{}", base.trim_end_matches('/'), path)
    }

    fn auth_header(&self) -> Option<String> {
        self.inner
            .token
            .read()
            .unwrap()
            .as_ref()
            .map(|t| format!("Bearer {}", t))
    }

    pub async fn me(&self) -> Result<UserMe> {
        let mut req = self.inner.http.get(self.url("/api/auth/me"));
        if let Some(h) = self.auth_header() {
            req = req.header("Authorization", h);
        }
        let res = req.send().await.context("GET /api/auth/me")?;
        if !res.status().is_success() {
            return Err(anyhow!("/api/auth/me {}", res.status()));
        }
        Ok(res.json().await.context("decode /me")?)
    }

    pub async fn pull(&self, since: i64, limit: i64) -> Result<SyncPullResponse> {
        let mut req = self
            .inner
            .http
            .get(self.url(&format!("/api/sync/pull?since={}&limit={}", since, limit)));
        if let Some(h) = self.auth_header() {
            req = req.header("Authorization", h);
        }
        let res = req.send().await.context("GET /api/sync/pull")?;
        match res.status() {
            StatusCode::OK => Ok(res.json().await.context("decode sync/pull")?),
            s => Err(anyhow!("sync/pull {}", s)),
        }
    }

    pub async fn list_reminders(&self) -> Result<Vec<ReminderRule>> {
        let mut req = self.inner.http.get(self.url("/api/reminders"));
        if let Some(h) = self.auth_header() {
            req = req.header("Authorization", h);
        }
        let res = req.send().await.context("GET /api/reminders")?;
        if !res.status().is_success() {
            return Err(anyhow!("/api/reminders {}", res.status()));
        }
        let body: RemindersListResponse = res.json().await.context("decode reminders")?;
        Ok(body.items)
    }

    pub async fn get_reminder(&self, id: i64) -> Result<ReminderRule> {
        let mut req = self
            .inner
            .http
            .get(self.url(&format!("/api/reminders/{}", id)));
        if let Some(h) = self.auth_header() {
            req = req.header("Authorization", h);
        }
        let res = req.send().await.context("GET /api/reminders/{id}")?;
        if !res.status().is_success() {
            return Err(anyhow!("/api/reminders/{} {}", id, res.status()));
        }
        Ok(res.json().await?)
    }
}
