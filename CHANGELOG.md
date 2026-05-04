# Changelog (top-level)

## v1.4.1 (2026-05-04) - Android 筛选与设置体验修复

- **Android:任务筛选逻辑对齐 Web**
  - 工作台改为日期范围筛选 + 状态筛选两层逻辑,支持今日、明天、本周、近 7 天、近 30 天、全部、有日期、无日期。
  - 「未完成」「已过期」「已完成」与 Web 保持互斥语义,切换日期筛选时会重新拉取对应服务端数据。
  - 右上角刷新和更多菜单移除,改为下拉刷新。
- **Android:设置与页面视觉优化**
  - 设置页新增时区选择弹窗,默认仍为 `Asia/Shanghai`,用户可手动改为其他 IANA 时区并持久化到服务端。
  - 设置页移除重复的「Android 通知与强提醒权限」卡片,权限入口保留在「提醒权限」页面。
  - 工作台移除顶部概览矩形卡片,专注与统计页面改为更现代的筛选式移动端布局。
- **版本:更新到 V1.4.1**
  - server / web / windows / android / release metadata 全部更新到 `1.4.1`。

## v1.4.0 (2026-05-04) - Telegram 风格底部导航与任务时长

- **Android:重做主入口导航与登录首屏**
  - 主入口改为底部导航:工作台 / 日程 / 专注 / 统计 / 我的。
  - 登录页改为产品介绍与登录一体首屏,去掉“服务端/认证中心”等面向技术人员的文案。
  - 任务编辑页的截止时间与提醒时间改为 Material3 日期选择器和时间盘。
- **三端:任务新增预计时长**
  - 服务端新增 `duration_minutes` 字段与迁移,Web / Windows / Android 创建、编辑、展示保持一致。
  - 预计时长限制在 0 到 1440 分钟,支持常用快捷选项和自定义分钟数。
- **下载:修复客户端下载入口**
  - Web 下载按钮改为使用 release 清单中的文件名,并在 Tauri 环境中通过系统浏览器打开下载链接。
  - release 清单更新到 `1.4.0`,构建产物继续统一发布到 `releases/`。
- **版本:更新到 V1.4.0**
  - server / web / windows / android / release metadata 全部更新到 `1.4.0`。

## v1.3.0 (2026-05-04) - 下载发布流程补充

- 客户端构建目标会自动把 Windows 和 Android 编译产物发布到 `releases/` 目录,避免下载链接指向不存在的文件。
- Web 客户端下载卡片改为从 `releases/latest.json` 读取文件名,不再写死 Windows 安装包文件名。
- 服务端 `/downloads/` 支持从仓库根目录或 `server/` 目录启动时正确解析 `releases/` 文件夹。

参见 `server/CHANGELOG.md` 的详细变更记录。本文件用于记录跨 server / web / android / windows 的整体里程碑。

## v1.3.0 (2026-05-04) — Android 产品化首页 + OIDC 邮箱与前台回拉

- **Android:主页面产品化重设计**
  - 首屏改为“今日工作台”:账号/同步状态、待办/今日/逾期/完成指标、日历/通知/番茄/统计快捷入口、Telegram 风格任务流。
  - 空状态从大面积空白改为有引导动作的产品卡片。
  - 登录页、设置页、任务编辑页及二级页面统一视觉密度与中文文案。
- **Android:错误统一弹窗**
  - 任务、登录、通知、Telegram、统计、番茄、日历、设置等页面的错误均改为弹窗提示。
  - Repository 层把常见服务端错误码与网络异常转为中文用户提示。
- **时区:用户可自动同步本机时区**
  - Android 设置页展示账户时区与本机时区差异,支持一键同步到本机时区。
  - 同步通过服务端 `PATCH /api/auth/me` 持久化,并更新本地登录会话。
- **OAuth/OIDC:真实邮箱兜底**
  - 服务端 OAuth 解析增加 OIDC `id_token` claims 兜底,优先使用其中的 `email` / `name` / `sub`。
  - 认证中心修复后,TaskFlow 会拿到真实邮箱,不再依赖占位邮箱。
- **Windows / Android:登录后主动回前台**
  - Windows Tauri 登录成功后调用窗口前置命令。
  - Android OAuth finalize 成功后主动拉起 `MainActivity`,避免登录完成后静默停在浏览器后台。
- **版本:更新到 V1.3.0**
  - server / web / windows / android / release metadata 全部更新到 `1.3.0`。

## v1.0.0 (2026-05-03) — 提醒链路修复 + 三端图标与时区统一

- **提醒:修复 Windows / Web 到点无提示**
  - Windows 登录或刷新 token 后立即执行一次完整同步,把服务端 reminder 镜像落到本地 SQLite,避免后台调度器空跑。
  - Web 在 Tauri 内新增提醒、启停提醒、删除提醒后主动触发 Windows 端同步,减少 30 秒轮询窗口造成的漏提醒体感。
  - Web SSE 改为使用统一 API base,并在 access token 过期时走现有 refresh 单飞逻辑后重连,避免 Tauri 绝对地址模式下通知流连错地址或持有旧 token。
- **时区:默认统一为中国上海**
  - 服务端、Web、Windows、Android 的用户 / todo / reminder 默认时区统一为 `Asia/Shanghai`。
  - 服务端统计接口在用户时区为空或异常时回退上海时区,不再回退 UTC。
- **账号:修复 QiShu OAuth 邮箱与显示名**
  - OAuth 邮箱字段增加 `email_name` / `emailName` / `email_address` / `user_email` / `preferred_username` 等兼容读取,优先展示 QiShu 返回的真实邮箱。
  - OAuth 再登录时只同步邮箱,不再覆盖用户在设置里手动修改过的显示名。
- **图标:三端统一清晰图标**
  - Web favicon 与 Windows 多尺寸 PNG / ICO 统一为侧边栏同风格的高分辨率图标。
  - Android launcher 背景色和前景勾形同步为新版品牌图标风格。
- **版本:定版 V1.0.0**
  - server / web / windows / android / release metadata 全部更新到 `1.0.0`。

## v0.5.0 (2026-05-03) — 管理面板 + Docker 部署

- **Windows:修复托盘恢复与 WebView2 启动目录**
  - 主窗口改为 `setup()` 阶段手动创建,并把 WebView2 数据目录固定到应用 `data/webview`,避免默认 `LOCALAPPDATA` 目录残留占用或权限异常导致启动失败。
  - 窗口 X 在 `CloseRequested` 阶段拦截并隐藏到托盘,避免主窗口被销毁后托盘"打开 TaskFlow"和双击快捷方式都无法恢复页面。
- **后端:管理员能力**
  - migration v5:`users` 加 `is_admin` / `is_disabled`,新表 `audit_logs`(管理员动作时间线)。
  - 启动时根据 `ADMIN_EMAIL` / `ADMIN_PASSWORD` 引导首位管理员 —— 已存在则提升,不存在则创建。已存在管理员的密码不会被 `.env` 默认值覆盖。
  - 新中间件 `RequireAdmin`;新路由 `/api/admin/*`(系统状态、用户管理、审计、数据清理、配置摘要)。所有写操作都会记一条审计。
  - `is_disabled` 用户在本地登录、refresh、OAuth finalize 三条入口都被拒绝,被禁用时会立即撤销其 refresh token。
  - 配置层支持 `TASKFLOW_JWT_SECRET` 等环境变量覆盖 TOML 中的敏感字段,便于容器化部署。
- **前端:管理面板视图**
  - 新视图 `views/Admin.vue`,五个 tab:系统状态 / 用户管理 / 审计日志 / 数据清理 / 系统设置。沿用既有 `settings-card` 风格,不弹层、不抽屉,就是普通的右侧主区域内容。
  - 仅 `is_admin` 账号能在左侧栏看到「管理」入口与 `ADMIN` 徽章,路由 + 视图双层 guard。
  - `is_admin` / `is_disabled` 字段贯穿 `User` 类型与 `useAuthStore`。
- **部署:Docker / docker-compose**
  - `server/Dockerfile`(distroless / nonroot / CGO=0 静态)+ `web/Dockerfile`(Vite 构建后 nginx 反代 `/api`、`/ws`)。
  - 仓库根新增 `docker-compose.yml` 与 `.env.example`,包含 `ADMIN_EMAIL=admin@example.com` / `ADMIN_PASSWORD=ChangeMeNow123!` 默认值。`docker compose up -d --build` 即可。

## v0.4.1 (2026-04-29)

补完 v0.4.0 留下的小坑:

- **Android 新增 Calendar 屏幕** —— 6×7 月历视图,每格显示该日待办数量(小圆点),可切换月份 / 跳转今天 / 选中某天看任务清单。补齐与 Web 端 `Calendar.vue` 的特性差。
- **Android Tasks 屏幕新增搜索** —— TopAppBar 多了搜索图标,按标题 / 描述本地过滤,清空按钮一键回到列表。
- **Android Pomodoro 实时倒计时** —— 进行中的 session 用大字体 MM:SS 倒计 + 进度条,到 0 时提示"⏰ 时间到 — 点完成结算"。
- **Android 自动跳转登录** —— 当后台 401 把 token 清空(AuthInterceptor 触发),`MainActivity` 的 `LaunchedEffect(session.isLoggedIn)` 把用户推回登录页,不再卡在死页。
- **`bootstrap.sh`** —— 给 Android 项目加了一键生成 `gradle-wrapper.jar` 的脚本,因为这个二进制文件不便随源码分发。
- 修复 `TodoEditViewModel.load()` 里残留的 `subtasks = ... .let { emptyList() }` 死代码。
- 删除 `TasksScreen` 起草时留下的两个空 placeholder Composable + `AlarmActivity` 的未使用 `isOnline` 状态变量。
- 顺手把 README 与 Android 子 README 同步成新版界面 + 限制清单。

## v0.4.0 (2026-04-29)

- **阶段 7 + 8:Android 原生客户端**
  - Kotlin 2.0 + Compose Material3 + Room + Retrofit + WorkManager
  - 11 个屏幕:Login / Register / Tasks(7 过滤器) / TodoEdit(子任务+提醒) / Notifications / Telegram / Stats / Pomodoro / Settings / PermissionCheck / AlarmActivity
  - **强提醒管线**:`AlarmManager.setExactAndAllowWhileIdle` 注册 → `AlarmReceiver` 唤起 → `AlarmForegroundService`(响铃 / 振动 / wake_lock,90 s 安全自停) + `AlarmActivity`(锁屏全屏 Compose 强提醒窗,Stop / Snooze / Complete)
  - `BootReceiver` 重启自动重排所有 active 提醒
  - `PermissionCheckScreen`(规格 §6):POST_NOTIFICATIONS / SCHEDULE_EXACT_ALARM / 全屏意图 / 电池白名单 5 项自检
  - 离线策略(规格 §4):Room 镜像最后一次同步状态;断网仍可触发已注册的本地提醒;UI 进入只读
  - Telegram 绑定通过 `tg://resolve?domain=…&start=bind_<token>` 深链(规格 §8),没有 chat_id / 密码 / 验证码输入框

- **阶段 9 + 10:Windows Tauri 客户端**
  - Tauri v2 + Rust + 复用 `web/` 的 Vue 前端(共享 95% UI 代码)
  - 本地 SQLite 镜像最近一次同步的 reminder 规则
  - Rust 后台:30 s sync 循环 + 5 s scheduler 循环
  - 强提醒:Windows Toast + 总在最前的 Alarm 窗口 + rodio 响铃(90 s 安全上限)
  - 系统托盘 + 关窗藏托盘 + tauri-plugin-autostart 开机自启

- **阶段 12:部署套件 (`deploy/`)**
  - 加固版 systemd unit(`NoNewPrivileges` / `ProtectSystem=strict` / `MemoryMax=512M`)
  - 生产 nginx HTTPS 配置(HSTS / CSP / 长连接 SSE / SPA fallback)
  - 一键 `install.sh`(创建用户、复制文件、systemd、nginx、certbot、cron 备份)
  - SQLite `VACUUM INTO` 备份脚本 + 14 天滚动 + 完整性校验
  - Telegram webhook 注册脚本

## v0.3.0 (2026-04-29)

- 阶段 6:Web 完整管理端(`web/`)。
- 阶段 11:后端番茄专注(`pomodoro_sessions` + 7 个端点) + 数据复盘(4 个 stats 端点)。
- 仓库结构调整:从 `cmd/internal/...` 平面布局改为 `server/` + `web/` 双根布局,顶层 `Makefile` 统一编排。

## v0.2.0 (2026-04-28)

- 阶段 4:Telegram Bot deep-link 绑定与推送。
- 阶段 5:服务端调度器、通知投递、SSE 实时推送。

## v0.1.0

- 阶段 1:后端骨架。
- 阶段 2:TODO / 列表 / 子任务 API。
- 阶段 3:RRULE 提醒规则。
