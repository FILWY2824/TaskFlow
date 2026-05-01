# TaskFlow Windows 客户端 (规格 v2.2 阶段 9 + 10)

> Tauri v2 + Rust + Vue 3 (复用 `../web/`)。
> 体积 ~7-10 MB(WebView2 不计),内存 ~30-60 MB。

## 端定位(规格 §2.3)

- **完整管理端**:复用 `../web/` 的 Vue 前端,所有 TODO / 列表 / 子任务 / 提醒 / 日历 / 番茄 / 复盘 / 通知 / Telegram 都直接可用。
- **本地强提醒**:Rust 进程托管,即使断网也按本地时间触发。
- **离线只读**(规格 §4):本地 SQLite 镜像最近一次同步的状态,断网时前端进入只读模式;不允许离线写。

## 目录结构

```
windows/
├── README.md
├── package.json                # 调起 Vite dev server / 复用 ../web src
├── vite.config.ts              # 复用 ../web 但产物输出到 dist/
├── index.html                  # 引 ../web/src/main.ts
├── src-tauri/
│   ├── Cargo.toml
│   ├── tauri.conf.json
│   ├── build.rs
│   ├── icons/                  # 图标(用占位 SVG,生成 .ico 由 tauri 构建时完成)
│   └── src/
│       ├── main.rs             # tauri::Builder + setup
│       ├── config.rs           # 用户偏好(server URL、access_token、autostart 等)
│       ├── db.rs               # 本地 SQLite 镜像
│       ├── api.rs              # HTTP client(reqwest),向服务端 pull/push
│       ├── sync.rs             # 启动后增量同步
│       ├── scheduler.rs        # 本地调度循环
│       ├── alarm.rs            # 强提醒窗口 / 响铃管理
│       ├── notify.rs           # Windows toast 通知
│       ├── tray.rs             # 系统托盘
│       └── commands.rs         # 暴露给前端的 #[tauri::command]
```

## 开发

需要 Windows 10+ + Visual Studio 生成工具 + WebView2 Runtime。

```bash
# 装 Rust
# https://rustup.rs/

# 第一次:把 ../web 装一遍依赖(Tauri dev 需要它的 Vite server)
cd ../web && npm install

cd ../windows
npm install
npm run tauri:dev
```

## 构建

```bash
npm run tauri:build
# 产物:src-tauri/target/release/bundle/{msi,nsis}/...
```

## 本地强提醒流程(规格 §7)

```
用户在前端创建 reminder
    ↓
Web 前端 POST /api/reminders → 服务端
    ↓
服务端写库 + 推 SSE event
    ↓
Tauri 后台 sync 拉到 reminder
    ↓
本地 SQLite 缓存 + scheduler 重新计算最近触发时间
    ↓
到点:
    1. Windows Toast 通知(可点 "完成 / 稍后 / 打开")
    2. 弹出强提醒窗口(总在最前 + 焦点)
    3. 循环播放本地铃声(可在窗口里点 "停止")
    4. 写本地投递日志
```

## 端到端断网验证

1. 启动 windows 客户端,登录,创建一个未来 2 分钟的提醒。
2. 关闭服务端 / 断网。
3. 等到点,Tauri 应当:
   - 弹 Windows 系统通知;
   - 弹强提醒窗口 + 响铃;
   - 用户点"完成"时,**仅**本地停止响铃,不提交到服务端(规格 §4 要求)。
4. 网络恢复后,用户在前端再点一次完成,状态同步到服务端。
