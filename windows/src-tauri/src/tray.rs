// System tray icon + menu.
//
// 注意:tauri.conf.json 中已移除 trayIcon 配置,完全由本文件以代码方式
// 创建托盘图标,避免 config 与代码各建一个托盘导致"双图标 + 菜单无效"。
//
// 左键单击 = 显示/隐藏主窗口
// 右键菜单 = 打开 / 退出
//
// 同步功能完全由后台静默执行,不再暴露"立即同步"按钮。

use tauri::{
    menu::{MenuBuilder, MenuItemBuilder},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager,
};
use std::sync::atomic::Ordering;

/// 把窗口可靠地拉到屏幕最前面。
/// Windows 对前台窗口有严格限制(ForegroundLockTimeout),单纯 set_focus()
/// 往往只是任务栏闪烁。这里用"短暂置顶 → 取消"的技巧绕过限制。
fn bring_window_to_front(win: &tauri::WebviewWindow) {
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
