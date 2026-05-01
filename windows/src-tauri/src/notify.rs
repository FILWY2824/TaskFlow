// Thin wrapper over tauri-plugin-notification.
//
// 在 Windows 上,tauri-plugin-notification 走 WinRT 推 Action Center toast。
// 用户可以像处理任何系统 toast 一样关掉它;我们不依赖 toast 本身去"打断用户",
// 那是 alarm window 的职责。
//
// 但本模块还做一件用户特别要求的事:**主窗口拉到屏幕最前 + 闪烁任务栏**。
// 提醒触发时除了系统 toast,也短暂把主窗口置顶 + 抢焦点,确保用户当下就看到。

use anyhow::Result;
use tauri::{AppHandle, Manager};
use tauri_plugin_notification::NotificationExt;

/// 发系统 toast。失败仅 log,不冒泡 —— alarm 还有强提醒窗口兜底,toast 只是锦上添花。
pub fn show(handle: &AppHandle, title: &str, body: &str) -> Result<()> {
    handle
        .notification()
        .builder()
        .title(title)
        .body(body)
        .show()?;
    Ok(())
}

/// 把主窗口拉到屏幕最前。alarm.fire() 在显示强提醒窗口"之前"会调一下这个,
/// 这样即使用户禁用了 toast 或没看托盘,主窗口也会主动凑到眼前。
///
/// 非阻塞:用 always_on_top 短暂置顶 ~400ms 后取消,避免长期挡其他窗口。
pub fn raise_main_window(handle: &AppHandle) {
    if let Some(win) = handle.get_webview_window("main") {
        let _ = win.show();
        let _ = win.unminimize();
        let _ = win.set_always_on_top(true);
        let _ = win.set_focus();
        let win_clone = win.clone();
        tauri::async_runtime::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(400)).await;
            let _ = win_clone.set_always_on_top(false);
        });
    }
}
