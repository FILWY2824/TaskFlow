// Thin wrapper over tauri-plugin-notification.
//
// On Windows, tauri-plugin-notification dispatches Action Center toasts
// via the WinRT API. The user can dismiss them like any other toast;
// we don't rely on the toast itself for the strong-reminder UX (that's
// the alarm window). The toast is just an immediate, low-friction
// "something happened" cue that survives if the alarm window fails to
// open for any reason.

use anyhow::Result;
use tauri::AppHandle;
use tauri_plugin_notification::NotificationExt;

pub fn show(handle: &AppHandle, title: &str, body: &str) -> Result<()> {
    handle
        .notification()
        .builder()
        .title(title)
        .body(body)
        .show()?;
    Ok(())
}
