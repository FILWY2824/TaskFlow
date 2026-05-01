// AlarmController:
//   1. Posts a Windows toast (immediate, can be dismissed).
//   2. Opens / focuses a dedicated full-screen alarm window
//      (label "alarm-{rule_id}"), always-on-top.
//   3. Spawns a rodio Sink that loops a built-in beep tone until told
//      to stop.
//   4. Keeps a registry of active alarms keyed by rule_id so the
//      `stop_alarm` IPC command can shut down ringing + close the window.
//
// The alarm window is a separate Webview window pointed at
// `/#/alarm?id=<rule_id>&title=<...>` — the Vue side renders the actual
// "停止 / 完成 / 稍后" UI.

use crate::db::CachedRule;
use crate::notify;
use anyhow::Result;
use chrono::{DateTime, Utc};
use rodio::source::{SineWave, Source};
use rodio::{OutputStream, Sink};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;
use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindowBuilder};

#[derive(Clone)]
pub struct AlarmController {
    inner: Arc<AlarmInner>,
}

struct AlarmInner {
    /// rule_id -> stop signal sender
    active: Mutex<HashMap<i64, std::sync::mpsc::Sender<()>>>,
}

impl AlarmController {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(AlarmInner {
                active: Mutex::new(HashMap::new()),
            }),
        }
    }

    /// Show toast + open alarm window + start ringing.
    pub async fn fire(&self, handle: &AppHandle, rule: CachedRule, fire_at: DateTime<Utc>) {
        let title = if rule.title.is_empty() {
            "TaskFlow 提醒".to_string()
        } else {
            rule.title.clone()
        };

        // 1. Toast — non-blocking and won't fail the pipeline if it errors.
        if let Err(e) = notify::show(handle, &title, "到点啦 ⏰") {
            log::warn!("toast for rule {} failed: {:#}", rule.id, e);
        }

        // 2. Alarm window — only when fullscreen flag is set on the rule.
        if rule.fullscreen {
            if let Err(e) = self.spawn_alarm_window(handle, &rule, fire_at) {
                log::warn!("alarm window for rule {} failed: {:#}", rule.id, e);
            }
        }

        // 3. Ringing
        if rule.channel_local {
            self.start_ringing(rule.id, &rule.ringtone);
        }
    }

    fn spawn_alarm_window(&self, handle: &AppHandle, rule: &CachedRule, fire_at: DateTime<Utc>) -> Result<()> {
        let label = format!("alarm-{}", rule.id);
        // 已经存在则只 focus
        if let Some(win) = handle.get_webview_window(&label) {
            let _ = win.show();
            let _ = win.set_focus();
            let _ = win.set_always_on_top(true);
            return Ok(());
        }
        let url = format!(
            "alarm?id={}&title={}&fire_at={}",
            rule.id,
            urlencoding::encode_or_lossy(&rule.title),
            fire_at.to_rfc3339(),
        );
        WebviewWindowBuilder::new(handle, label.clone(), WebviewUrl::App(url.into()))
            .title("TaskFlow — 提醒")
            .inner_size(520.0, 360.0)
            .center()
            .always_on_top(true)
            .focused(true)
            .resizable(false)
            .skip_taskbar(false)
            .build()?;
        Ok(())
    }

    /// Spin up a background thread that loops a sine wave until killed.
    /// We use a SineWave at 880 Hz / 0.5 s on / 0.5 s off to be both
    /// audible and not too jarring. A future version could load a user-
    /// configurable .wav file from disk.
    fn start_ringing(&self, rule_id: i64, _ringtone: &str) {
        // Reuse if already ringing
        {
            let active = self.inner.active.lock().unwrap();
            if active.contains_key(&rule_id) {
                return;
            }
        }
        let (tx, rx) = std::sync::mpsc::channel::<()>();
        {
            let mut active = self.inner.active.lock().unwrap();
            active.insert(rule_id, tx);
        }

        thread::spawn(move || {
            // OutputStream and Sink must live for the entire ringing duration.
            let stream = match OutputStream::try_default() {
                Ok((stream, handle)) => Some((stream, handle)),
                Err(e) => {
                    log::warn!("ringing: no audio device: {:#}", e);
                    None
                }
            };
            let sink = stream.as_ref().and_then(|(_s, h)| Sink::try_new(h).ok());

            // Auto-stop after 90s as a safety net (no infinite ringing if the
            // user goes AFK).
            let max_duration = Duration::from_secs(90);
            let start = std::time::Instant::now();
            loop {
                if rx.try_recv().is_ok() {
                    break;
                }
                if start.elapsed() > max_duration {
                    log::info!("ringing: rule {} auto-stopped after timeout", rule_id);
                    break;
                }
                if let Some(sink) = &sink {
                    let beep = SineWave::new(880.0)
                        .take_duration(Duration::from_millis(500))
                        .amplify(0.20);
                    sink.append(beep);
                    // small gap
                    thread::sleep(Duration::from_millis(500));
                } else {
                    // no audio — fall back to 1s sleep ticks
                    thread::sleep(Duration::from_millis(1000));
                }
            }
            if let Some(sink) = sink {
                sink.stop();
            }
        });
    }

    pub fn stop(&self, handle: &AppHandle, rule_id: i64) {
        let label = format!("alarm-{}", rule_id);
        if let Some(win) = handle.get_webview_window(&label) {
            let _ = win.close();
        }
        let mut active = self.inner.active.lock().unwrap();
        if let Some(tx) = active.remove(&rule_id) {
            let _ = tx.send(());
        }
    }
}

// 极轻量 URL 编码,避免引入 urlencoding crate。仅处理常见特殊字符。
mod urlencoding {
    pub fn encode_or_lossy(s: &str) -> String {
        let mut out = String::with_capacity(s.len());
        for b in s.bytes() {
            match b {
                b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                    out.push(b as char)
                }
                _ => {
                    out.push('%');
                    out.push_str(&format!("{:02X}", b));
                }
            }
        }
        out
    }
}
