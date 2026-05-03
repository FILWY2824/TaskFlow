// System tray icon + menu.
//
// 注意:tauri.conf.json 中已移除 trayIcon 配置,完全由本文件以代码方式
// 创建托盘图标,避免 config 与代码各建一个托盘导致"双图标 + 菜单无效"。
//
// 左键单击 = 显示/隐藏主窗口
// 右键菜单 = 打开 / 退出
//
// 同步功能完全由后台静默执行,不再暴露"立即同步"按钮。

use std::sync::atomic::Ordering;
use tauri::{
    menu::{MenuBuilder, MenuItemBuilder},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager,
};

/// 把窗口可靠地拉到屏幕最前面。
///
/// Windows 对前台窗口有严格限制(ForegroundLockTimeout),单纯 set_focus()
/// 往往只是任务栏闪烁。这里用纯 Win32 API 同步操作,不依赖 Tauri 的
/// 异步 set_always_on_top(异步导致时序不可控):
///   1. ShowWindow(SW_SHOW + SW_RESTORE) — 确保窗口可见且非最小化;
///   2. SetWindowPos(HWND_TOPMOST) — 同步置顶,Windows 立即生效;
///   3. SetForegroundWindow — 抢焦点(置顶状态可绕过前台锁定);
///   4. 500ms 后 SetWindowPos(HWND_NOTOPMOST) — 取消置顶,恢复常态。
#[cfg(windows)]
pub(crate) fn bring_window_to_front(win: &tauri::WebviewWindow) {
    use windows::Win32::UI::WindowsAndMessaging::{
        SetForegroundWindow, SetWindowPos, ShowWindow,
        HWND_TOPMOST, SWP_NOMOVE, SWP_NOSIZE, SW_RESTORE, SW_SHOWNORMAL,
    };

    // 先确保窗口可见(处理 win.hide() 的情况)
    let _ = win.show();
    let _ = win.unminimize();

    if let Ok(hwnd) = win.hwnd() {
        unsafe {
            // ShowWindow(SW_SHOWNORMAL) 强制显示(处理 hide 后状态不一致)
            _ = ShowWindow(hwnd, SW_SHOWNORMAL);
            // SW_RESTORE 从最小化恢复
            _ = ShowWindow(hwnd, SW_RESTORE);
            // 同步置顶 —— 不依赖 Tauri 的异步 set_always_on_top
            _ = SetWindowPos(hwnd, Some(HWND_TOPMOST), 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE);
            // 置顶状态下 SetForegroundWindow 可绕过前台锁定
            _ = SetForegroundWindow(hwnd);
        }
        // 500ms 后取消置顶(HWND 不可 Send,用 Tauri 异步 API 降级处理)
        let win_clone = win.clone();
        tauri::async_runtime::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(500)).await;
            let _ = win_clone.set_always_on_top(false);
        });
    } else {
        // 降级:获取 HWND 失败时用跨平台方法
        let _ = win.set_always_on_top(true);
        let _ = win.set_focus();
        let win_clone = win.clone();
        tauri::async_runtime::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(400)).await;
            let _ = win_clone.set_always_on_top(false);
        });
    }
}

#[cfg(not(windows))]
pub(crate) fn bring_window_to_front(win: &tauri::WebviewWindow) {
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

pub fn setup(app: &mut tauri::App) -> tauri::Result<()> {
    let item_open = MenuItemBuilder::with_id("open", "打开 TaskFlow").build(app)?;
    let item_quit = MenuItemBuilder::with_id("quit", "退出 TaskFlow").build(app)?;
    let menu = MenuBuilder::new(app)
        .item(&item_open)
        .separator()
        .item(&item_quit)
        .build()?;

    let mut builder = TrayIconBuilder::with_id("main-tray").tooltip("TaskFlow");
    if let Some(icon) = app.default_window_icon().cloned() {
        builder = builder.icon(icon);
    }
    let _tray = builder
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app_handle, event| match event.id().as_ref() {
            "open" => {
                if let Some(win) = app_handle.get_webview_window("main") {
                    bring_window_to_front(&win);
                }
            }
            "quit" => {
                crate::QUIT_FLAG.store(true, Ordering::SeqCst);
                app_handle.exit(0);
            }
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let app_handle = tray.app_handle();
                if let Some(win) = app_handle.get_webview_window("main") {
                    let visible = win.is_visible().unwrap_or(false);
                    if visible {
                        let _ = win.hide();
                    } else {
                        bring_window_to_front(&win);
                    }
                }
            }
        })
        .build(app)?;

    Ok(())
}
