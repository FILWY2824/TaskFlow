# TaskFlow — Codex 项目指南

> 多用户 TODO + Android / Windows 强提醒 + 全端管理系统
> 当前版本:**v1.3.0** (开发中)|规格文档 v2.2 已完成 12 个阶段
> 本文件是 Codex 启动时自动加载的项目记忆,**所有 subagent 都默认共享这里的内容**

---

## 仓库速览

```
taskflow/
├── server/        # Go 1.22 + SQLite WAL + JWT(后端,真理之源)
├── web/           # Vue 3 + Vite + Pinia + TypeScript
├── windows/       # Tauri v2(Rust)— 复用 ../web 源码
├── android/       # Kotlin 2.0 + Compose Material3 + Room
├── deploy/        # systemd / nginx / install.sh / 备份脚本
├── docker-compose.yml
├── Makefile       # 顶层编排
└── CHANGELOG.md
```

完整端定位见仓库根 `README.md`、运维细节见 `deploy/README.md`。

---

## 核心架构原则

1. **服务端永远是 source of truth**。Windows / Android 的本地 SQLite 只是镜像。
2. **离线策略**:断网时已注册的 reminder **照常本地触发**(AlarmManager / Tauri scheduler);但禁止任何写操作 —— 离线时永远不在本地把 todo 标完成,避免下次同步覆盖服务端。
3. **强提醒**只在 Windows / Android 上有效。Web 浏览器沙箱里**没有**真正的强提醒,Web 不要尝试模拟。
4. **JWT**:access 15 min / refresh 30 d,bcrypt cost = 11。所有破坏性操作必须校验所有权 (`user_id` 匹配)。
5. **DB schema 迁移**:写在 `server/internal/db/db.go` 的 `applyMigrations()` 里,序号递增,**永不修改已发布的 migration**。当前最高 v5。

---

## 关键技术决策(改动前先看)

### 后端 (server/)

- 使用 `modernc.org/sqlite`(纯 Go 实现),**不是** `mattn/go-sqlite3`。这意味着 `CGO_ENABLED=0` 静态编译,部署不依赖 libc。
- 路由用 Go 1.22 原生 `http.ServeMux`,**不引入** gin / echo / chi。模式形如 `mux.HandleFunc("POST /api/todos", ...)`。
- RRULE 用 `github.com/teambition/rrule-go` 解析,`server/internal/rrule/rrule.go` 封装了 next-fire 计算。
- SSE 推送通过 `internal/events/hub.go` 的进程内 hub,per-user 广播,non-blocking。
- 调度器(`internal/scheduler/`)默认每 5 秒扫一次到期 reminder,投递后推进 `next_fire_at`。
- TOML 配置可被环境变量覆盖,见 `internal/config/env.go`(部署用)。

### Web (web/)

- Vue 3 `<script setup>` + Pinia + vue-router,**不用** Vuex、不用 Options API、不用 Element Plus 等组件库 —— 所有 UI 自己写在 `components/` 与 `views/` 里。
- 全部样式集中在 `src/style.css`(72K 单文件,分区域注释),**不用** Tailwind / SCSS。新增样式追加到对应区域。
- API 客户端在 `src/api.ts`(单文件),所有 fetch 都走那里,带 401 自动 refresh + 单飞。
- localStorage key 一律 `taskflow.*` 前缀。
- Tauri 检测在 `src/tauri.ts`;Tauri 模式下 API base 切到绝对 URL。
- TS 类型在 `src/types.ts`,与后端 `models/models.go` 的 JSON tag **必须保持一致**。

### Windows (windows/)

- Tauri v2,Rust 1.75+。`vite.config.ts` 把 `root` 指向 `../web`,alias `@` 指向 `../web/src`,**不复制 web 源码**。
- 本地 SQLite 用 `rusqlite`(同步 API),**不用** SQLx —— Tauri 编译时间太长。
- 响铃用 `rodio`,90 秒安全自停。
- Windows API 仅在 `cfg(windows)` 下引入 `windows` crate(MessageBoxW 致命错误 + 窗口置顶)。
- 后台任务:30s sync 循环 + 5s scheduler 循环,见 `src-tauri/src/scheduler.rs`、`sync.rs`。

### Android (android/)

- Kotlin 2.0,Compose 编译器由 Kotlin 自带(`alias(libs.plugins.kotlin.compose)`),不需要单独对齐版本。
- **不用 Hilt**,自管 DI(单例 `AppContainer`)—— 减少注解处理与 incremental 编译复杂度。
- Room 用 **KSP**(KAPT 已弃用)。
- Moshi 用 `@JsonClass(generateAdapter = true)` + codegen,**0 反射**。
- Token 存 `EncryptedSharedPreferences`(AES-256-GCM,主密钥在 Keystore)。
- 强提醒管线:`AlarmManager.setExactAndAllowWhileIdle` → `AlarmReceiver` → `AlarmForegroundService`(响铃/振动/wake_lock)+ `AlarmActivity`(锁屏全屏)。
- `BootReceiver` 重启自动重排所有 active 提醒。
- 明文 HTTP 仅 debug 构建对 `10.0.2.2 / 127.0.0.1` 开放,见 `res/xml/network_security_config.xml`。

---

## 常用命令

| 任务 | 命令 |
| --- | --- |
| 启动后端 | `make server-run` 或 `cd server && make run` |
| 启动 Web 开发 | `make web-dev`(http://localhost:5173) |
| 后端单测 | `make server-test` 或 `cd server && go test ./...` |
| Web 类型检查 | `make web-typecheck`(等价 `vue-tsc --noEmit`) |
| 全量构建 | `make build`(server + web) |
| Linux amd64 构建 | `make build-linux-amd64` |
| Tauri 开发 | `make windows-dev`(需 Rust + WebView2) |
| Tauri 打包 | `make windows-build` |
| Android debug | `make android-debug` |
| 源码 tarball | `make dist-src` |
| 部署 tarball | `make dist` |
| 清理 | `make clean` |

---

## 编码约定

- **注释/文档语言用中文**。代码标识符用英文。本项目所有 README、注释、CHANGELOG 都是中文,**保持一致**。
- 提交 message 用中文也可,但语气要简洁、客观,描述「做了什么+为什么」。参考 `CHANGELOG.md` 的语气。
- Go 代码 `gofmt -s` 强制格式化;`go vet` 必须过。
- TS 代码必须过 `vue-tsc --noEmit`(无 any 滥用、无未使用导入)。
- Kotlin / Rust 遵循官方默认风格(`ktlint` / `cargo fmt`)。

---

## 目录访问准则

- **改后端**:只动 `server/`。schema 改动必须新增 migration,不改老的。
- **改前端**:大概率只动 `web/`。Tauri 版本会自动复用,但如果用了仅浏览器 API 要在 `src/tauri.ts` 里做能力探测降级。
- **改 Android**:只动 `android/app/src/main/`。版本号在 `app/build.gradle.kts`。
- **改 Windows**:Rust 部分在 `windows/src-tauri/src/`,前端共用所以无需复制。
- **改部署**:`deploy/` + `docker-compose.yml` + `.env.example`。改动后**手动同步** `deploy/README.md`。

---

## ⚠️ 容易踩的坑

1. **CHANGELOG 别忘记更新**。每次有用户可见的改动都要在 `CHANGELOG.md`(顶层)+ 对应子项目的 CHANGELOG 里加一条。
2. **types.ts ↔ models.go**:任何 JSON 字段名/类型变动两边必须同步,否则前端解析挂。
3. **数据库迁移不可回退**:已发布的 migration 永远不要改。要修就加新的 v(N+1)。
4. **localStorage 键名**:已锁定 `taskflow.*`,改名会让现有用户看似"被登出"。要改必须写迁移代码。
5. **Tauri 不允许 `tauri://localhost` 跨域 fetch**:绝对 URL 要先 `setApiBase("https://...")`。
6. **Android 离线写入**:任何 ViewModel 在网络不可达时**绝不能**修改本地 Room 的 todo 状态(尤其是 `is_completed`)。仅停响铃 + 提示用户联网后再确认。
7. **Telegram webhook**:必须用 `secret_token` HTTP header,后端 constant-time 比较(`internal/handlers/telegram.go`)。
8. **OAuth `OAUTH_AUTHORIZE_URL`** 末尾必须有 `/#/`(Profile 项目用 hash 路由),否则 404。详见 `FIXES.md`。

---

## subagent 路由

本项目根据领域配置了若干 subagent(在 `.Codex/agents/` 下)。Codex 应该按下面规则委派:

| 任务关键词 | 委派给 | 描述 |
| --- | --- | --- |
| Go / handler / store / scheduler / RRULE / SQLite | `backend-go-dev` | 后端业务逻辑、API、调度、迁移 |
| Vue / Pinia / view / 前端样式 / api.ts / types.ts | `web-vue-dev` | Web 端 + Tauri 共用前端 |
| Kotlin / Compose / Room / AlarmManager / 强提醒 | `android-dev` | Android 原生客户端 |
| Rust / Tauri / rusqlite / rodio / Windows API | `windows-tauri-dev` | Windows 桌面客户端 |
| systemd / nginx / Docker / install.sh / 部署 | `deploy-ops` | 运维与部署套件 |
| 跨平台一致性检查、PR review | `code-reviewer` | 代码审查 |

复杂任务(如"加一个新字段贯穿所有端")请按 后端 → Web → Android → Windows 的顺序委派多个 agent,因为后端是真理之源。
