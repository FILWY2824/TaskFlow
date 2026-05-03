// System tray icon + menu.
//
// 注意:tauri.conf.json 中已移除 trayIcon 配置,完全由本文件以代码方式
// 创建托盘图标,避免 config 与代码各建一个托盘导致"双图标 + 菜单无效"。
//
// 左键单击 = 显示/隐藏主窗口
// 右键菜单 = 打开 / 立即同步 / 退出

use tauri::{
    menu::{MenuBuilder, MenuItemBuilder},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager,
};

pub fn setup(app: &mut tauri::App) -> tauri::Result<()> {
    let item_open = MenuItemBuilder::with_id("open", "打开 TaskFlow").build(app)?;
    let item_sync = MenuItemBuilder::with_id("sync", "立即同步").build(app)?;
    let item_quit = MenuItemBuilder::with_id("quit", "退出 TaskFlow").build(app)?;
    let menu = MenuBuilder::new(app)
        .item(&item_open)
        .item(&item_sync)
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
                    let _ = win.show();
                    let _ = win.unminimize();
                    let _ = win.set_focus();
                }
            }
            "sync" => {
                let handle = app_handle.clone();
                tauri::async_runtime::spawn(async move {
                    let state = handle.state::<crate::AppState>();
                    if let Err(e) = crate::sync::run_once(&state.api, &state.db).await {
                        log::warn!("manual sync failed: {:#}", e);
                    }
                });
            }
            "quit" => {
                std::process::exit(0);
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
                        let _ = win.show();
                        let _ = win.unminimize();
                        let _ = win.set_focus();
                    }
                }
            }
        })
        .build(app)?;

    Ok(())
}
