// TaskFlow Windows desktop entry point.
//
// Boot order:
//   1. Install panic hook + file logger (so闪退也留痕)。
//   2. Initialize logging.
//   3. Open local SQLite cache (created on first run).
//   4. Load AppConfig from disk (server URL, tokens, autostart preference).
//   5. Spawn background tasks:
//       - Sync loop: pulls /api/sync/pull every 30 s when token present.
//       - Scheduler loop: every 5 s scans local reminder cache, fires
//         strong-reminders that hit their next_fire_at.
//   6. Build Tauri tray + main window + register #[tauri::command] handlers.
//
// 闪退诊断(本版重点):
//   release 构建带 windows_subsystem = "windows",没有控制台 —— 任何 panic
//   或 expect() 失败都会静默退出,用户看到的就是"双击图标没反应"。
//
//   本文件:
//     a) 在 main 第一行就装 panic hook,把 panic 写到 %APPDATA%\TaskFlow\fatal.log;
//     b) 关键启动步骤(创建目录 / 打开 db / 读配置)用 try_init() 包起来,
//        失败时调 fatal_dialog() 弹 Windows MessageBox 并写日志再 exit(1),
//        这样用户至少能看到错误信息复制给我们;
//     c) env_logger 同时写到 stderr 与 fatal.log。
//
// Strong-reminder pipeline (offline-safe by design — see spec §4 / §7):
//   scheduler::tick() -> alarm::fire(rule):
//       1. Show Windows toast via tauri-plugin-notification.
//       2. Open / focus a dedicated full-screen alarm window
//          (label "alarm-<rule_id>"), always-on-top.
//       3. Start ringing on a background rodio Sink.
//       4. Window publishes a "stop" event; main process kills sink and
//          closes window. If user clicked "complete", we call the API only
//          if online — offline only stops the local ring (spec §4).

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod alarm;
mod api;
mod commands;
mod config;
mod db;
mod notify;
mod scheduler;
mod sync;
mod tray;

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use tauri::{Manager, RunEvent};
use tokio::sync::Mutex;

/// 全局退出标志。托盘"退出"或前端 quit_app 命令先把它设为 true,
/// 再调 app.exit(0)。ExitRequested 回调检查此标志:
///   - true  → 用户明确要退出,放行;
///   - false → 用户只是点了窗口 X,藏到托盘继续后台运行。
pub static QUIT_FLAG: AtomicBool = AtomicBool::new(false);

/// 另一个实例启动了(用户双击桌面图标 / 任务栏图标)。
/// 后台线程检测到 TCP 连接后设为 true;setup 里的 async task 轮询此标志
/// 并调 bring_window_to_front 把主窗口拉回前台。
static INSTANCE_SIGNAL: AtomicBool = AtomicBool::new(false);

pub struct AppState {
    pub cfg: Mutex<config::AppConfig>,
    pub db: Arc<db::LocalDb>,
    pub api: api::ApiClient,
    pub alarm: alarm::AlarmController,
}

fn main() {
    // 0) 单实例 + 激活信号:通过 TCP 端口绑定防止多开。
    //    第一个实例:绑定 127.0.0.1:19830 并启动后台线程监听来自
    //      第二个实例的连接;收到连接 → 设 INSTANCE_SIGNAL=true,
    //      setup 里的 async task 轮询后调 bring_window_to_front。
    //    第二个实例:连接端口并写一个字节通知第一个实例"把窗口拉回前台",
    //      然后静默退出。
    match std::net::TcpListener::bind("127.0.0.1:19830") {
        Ok(listener) => {
            // 第一个实例 — 占用端口并监听激活信号。
            // 把 listener 交给后台线程,让它一直活到进程结束(线程不结束,
            // listener 就不 drop,端口一直被占用)。
            listener.set_nonblocking(true).ok();
            std::thread::Builder::new()
                .name("instance-watch".into())
                .spawn(move || loop {
                    match listener.accept() {
                        Ok((_stream, _addr)) => {
                            INSTANCE_SIGNAL.store(true, Ordering::SeqCst);
                        }
                        Err(ref e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                            std::thread::sleep(std::time::Duration::from_millis(500));
                        }
                        Err(e) => {
                            log::warn!("instance listener error: {e}, restarting accept loop");
                            std::thread::sleep(std::time::Duration::from_millis(500));
                        }
                    }
                })
                .ok();
        }
        Err(_) => {
            // 第二个实例 — 通知第一个实例把窗口拉到前台,然后退出。
            if let Ok(mut stream) = std::net::TcpStream::connect("127.0.0.1:19830") {
                use std::io::Write;
                let _ = stream.write(&[1]);
                let _ = stream.flush();
            }
            return;
        }
    }

    // 1) 解析 app dir。失败的话连日志都没地方写,直接 MessageBox 退出。
    let app_dir = match config::default_app_dir() {
        Ok(d) => d,
        Err(e) => {
            fatal_dialog(
                None,
                &format!(
                    "无法定位应用数据目录: {e}\n\n通常因为 %APPDATA% 环境变量丢失。\n请在终端运行 `echo %APPDATA%` 检查,然后联系开发者。"
                ),
            );
            return;
        }
    };

    // 2) 创建 app dir + 装 panic hook(panic 写到 %APPDATA%\TaskFlow\fatal.log)。
    if let Err(e) = std::fs::create_dir_all(&app_dir) {
        fatal_dialog(
            None,
            &format!(
                "无法创建应用数据目录 {}: {e}\n\n可能被杀毒软件或权限拦截。",
                app_dir.display()
            ),
        );
        return;
    }
    install_panic_hook(app_dir.clone());

    // 3) 日志:同时写 stderr 与 fatal.log,默认 info 级别。
    init_logger(&app_dir);
    log::info!("TaskFlow 启动,app_dir = {}", app_dir.display());

    // 4) 打开本地 SQLite。这一步在某些防病毒/受限环境会失败,失败一定要让用户看到。
    let db = match db::LocalDb::open(&app_dir.join("cache.db")) {
        Ok(d) => Arc::new(d),
        Err(e) => {
            log::error!("open local db failed: {e:#}");
            fatal_dialog(
                Some(&app_dir),
                &format!(
                    "本地数据库初始化失败:\n  {e:#}\n\n请尝试:\n  1) 关闭杀毒软件再试;\n  2) 删除 {} 后重启程序。",
                    app_dir.join("cache.db").display()
                ),
            );
            return;
        }
    };

    // 5) 读配置(读不到默认值,不会失败)+ 构造 ApiClient + AlarmController。
    let cfg = config::AppConfig::load_or_default(&app_dir);
    let api = api::ApiClient::new(cfg.server_url.clone(), cfg.access_token.clone());
    let alarm = alarm::AlarmController::new();

    let state = AppState {
        cfg: Mutex::new(cfg),
        db: db.clone(),
        api,
        alarm: alarm.clone(),
    };

    // 6) 构造 tauri::Builder。如果 build() 失败(图标错、capabilities 错、context 错),
    //    依旧弹窗给用户看 —— 千万别用 expect() 静默退出。
    let app_dir_for_dialog = app_dir.clone();
    let app_result = tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_os::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            Some(vec!["--minimized"]),
        ))
        .manage(state)
        .invoke_handler(tauri::generate_handler![
            commands::set_server_config,
            commands::get_server_config,
            commands::get_default_server_url,
            commands::set_tokens,
            commands::clear_tokens,
            commands::set_autostart,
            commands::is_autostart_enabled,
            commands::sync_now,
            commands::stop_alarm,
            commands::open_external,
            commands::bring_window_to_front,
            commands::quit_app,
        ])
        .setup(move |app| {
            // Tray icon + menu —— 失败时只 log,不 panic(没有托盘也能用)。
            if let Err(e) = tray::setup(app) {
                log::warn!("tray setup failed: {e:#}");
            }

            // Background tasks
            let handle = app.handle().clone();
            let db_for_sync = db.clone();
            let db_for_sched = db.clone();

            // Sync loop
            tauri::async_runtime::spawn(async move {
                sync::run_sync_loop(handle, db_for_sync).await;
            });

            // Scheduler loop
            let handle2 = app.handle().clone();
            let alarm2 = alarm.clone();
            tauri::async_runtime::spawn(async move {
                scheduler::run_scheduler_loop(handle2, db_for_sched, alarm2).await;
            });

            // Instance activation poller — 当用户双击桌面/任务栏图标时,
            // 第二个实例通过 TCP 通知我们;这里每 300ms 检查一次信号并把
            // 主窗口拉回前台。
            let handle3 = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                loop {
                    tokio::time::sleep(std::time::Duration::from_millis(300)).await;
                    if INSTANCE_SIGNAL.swap(false, Ordering::SeqCst) {
                        log::info!("instance activation signal received, bringing window to front");
                        if let Some(win) = handle3.get_webview_window("main") {
                            tray::bring_window_to_front(&win);
                        } else {
                            log::warn!("main window not found for instance activation");
                        }
                    }
                }
            });

            Ok(())
        })
        .build(tauri::generate_context!());

    let app = match app_result {
        Ok(a) => a,
        Err(e) => {
            log::error!("tauri build failed: {e:#}");
            fatal_dialog(
                Some(&app_dir_for_dialog),
                &format!(
                    "Tauri 初始化失败:\n  {e:#}\n\n这通常是 WebView2 Runtime 缺失。\n请到 https://developer.microsoft.com/microsoft-edge/webview2/ 下载并安装 'Evergreen Standalone Installer'。"
                ),
            );
            return;
        }
    };

    // Hide window when user clicks the X — keep app running in tray.
    // User can fully quit via tray "退出" or commands::quit_app.
    app.run(|app_handle, event| match event {
        RunEvent::ExitRequested { api, .. } => {
            if QUIT_FLAG.load(Ordering::SeqCst) {
                // 用户明确要求退出(托盘菜单 / 前端 quit 命令),放行。
                log::info!("exit requested with QUIT_FLAG — shutting down");
                // 不调 prevent_exit(),Tauri 将正常走完清理流程后退出。
            } else {
                // 窗口关闭按钮 → 只藏到托盘,不退出。
                if let Some(win) = app_handle.get_webview_window("main") {
                    let _ = win.hide();
                }
                api.prevent_exit();
            }
        }
        _ => {}
    });
}

// ============================================================
// 启动期诊断:panic hook + 日志 + Windows MessageBox
// ============================================================

fn install_panic_hook(app_dir: PathBuf) {
    let prev = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let msg = format!(
            "[{}] PANIC: {}\nlocation: {}\n\n",
            chrono::Utc::now().to_rfc3339(),
            info.payload()
                .downcast_ref::<&str>()
                .copied()
                .or_else(|| info.payload().downcast_ref::<String>().map(|s| s.as_str()))
                .unwrap_or("(opaque payload)"),
            info.location()
                .map(|l| format!("{}:{}", l.file(), l.line()))
                .unwrap_or_else(|| "unknown".into()),
        );
        // 写日志文件
        let _ = append_fatal_log(&app_dir, &msg);
        // 弹窗 — 保险起见放在 panic hook 里,让用户知道发生了什么
        fatal_dialog(Some(&app_dir), &format!("TaskFlow 异常退出。\n\n{}", msg));
        // 然后再走原来的 hook(不写也能,可能多打一遍 stderr)
        prev(info);
    }));
}

fn init_logger(app_dir: &std::path::Path) {
    use env_logger::{Builder, Env};
    use std::io::Write;

    let log_path = app_dir.join("taskflow.log");
    // 滚动:每次启动追加;文件大于 5MB 时清空(简单滚动)。
    if let Ok(meta) = std::fs::metadata(&log_path) {
        if meta.len() > 5 * 1024 * 1024 {
            let _ = std::fs::remove_file(&log_path);
        }
    }
    let file = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(&log_path)
        .ok();

    let mut builder = Builder::from_env(Env::default().default_filter_or("info"));
    builder.format(move |buf, record| {
        let line = format!(
            "[{}] {:<5} [{}] {}\n",
            chrono::Local::now().format("%Y-%m-%d %H:%M:%S%.3f"),
            record.level(),
            record.target(),
            record.args(),
        );
        // 写文件(最佳努力)
        if let Some(mut f) = file.as_ref().and_then(|f| f.try_clone().ok()) {
            let _ = f.write_all(line.as_bytes());
        }
        // 写 stderr(release+windows_subsystem 下不可见,但 dev 模式有用)
        write!(buf, "{}", line)
    });
    let _ = builder.try_init();
}

fn append_fatal_log(app_dir: &std::path::Path, msg: &str) -> std::io::Result<()> {
    use std::io::Write;
    let path = app_dir.join("fatal.log");
    let mut f = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(&path)?;
    f.write_all(msg.as_bytes())
}

/// Windows 上弹一个 MessageBox 把致命错误告诉用户 + 提示日志位置。
/// 非 Windows 平台:走 stderr。
fn fatal_dialog(app_dir: Option<&std::path::Path>, body: &str) {
    let mut full = String::from(body);
    if let Some(dir) = app_dir {
        full.push_str(&format!(
            "\n\n详细日志已写入:\n  {}\n  {}",
            dir.join("fatal.log").display(),
            dir.join("taskflow.log").display(),
        ));
    }

    #[cfg(windows)]
    {
        use windows::core::PCWSTR;
        use windows::Win32::UI::WindowsAndMessaging::{
            MessageBoxW, MB_ICONERROR, MB_OK, MB_SETFOREGROUND, MB_TOPMOST,
        };
        let title: Vec<u16> = "TaskFlow 启动失败"
            .encode_utf16()
            .chain(std::iter::once(0))
            .collect();
        let body_w: Vec<u16> = full.encode_utf16().chain(std::iter::once(0)).collect();
        unsafe {
            let _ = MessageBoxW(
                None,
                PCWSTR(body_w.as_ptr()),
                PCWSTR(title.as_ptr()),
                MB_OK | MB_ICONERROR | MB_TOPMOST | MB_SETFOREGROUND,
            );
        }
    }
    #[cfg(not(windows))]
    {
        eprintln!("[FATAL] {}", full);
    }
}
