# Changelog

## v1.4.1 (2026-05-04) — Android 筛选与版本清单

- 后端版本号更新到 `1.4.1`,用于 `/healthz` 和发布元数据对齐。
- 本次主要为 Android 客户端体验更新,后端任务/时区接口沿用既有 `PATCH /api/auth/me` 与 todo 列表筛选能力。
- TODO 列表筛选、排序和统计的任务时间语义切换为 `start_at`,旧 `due_at` 只作为历史请求兜底。
- 新增 migration v8,为 `todos(user_id, start_at)` 添加索引。
- 今日 / 本周 / 近 7 天 / 近 30 天等复合筛选会保留在当前区间完成的历史过期任务,确保完成后进入“已完成”内容而不是消失。
- 新增 store 层测试覆盖 `start_at` 筛选和完成历史任务后的归属。

## v1.4.0 (2026-05-04) — 任务预计时长

### 新增

- TODO 模型新增 `duration_minutes`,用于记录任务预计持续时间。
- 数据库新增 migration v7,为 `todos` 表追加 `duration_minutes INTEGER NOT NULL DEFAULT 0`。
- 创建与更新 TODO 时校验预计时长必须在 0 到 1440 分钟之间。

### 测试

- 新增 store 层测试覆盖创建与更新时持久化 `duration_minutes`。

## v1.3.0 (2026-05-04) — OIDC 邮箱 + 时区同步

### 新增

- OAuth token response 支持解析 OIDC `id_token`,并用 `email` / `name` / `sub` 兜底 userinfo 缺失字段。
- Android 可通过既有 `PATCH /api/auth/me` 持久化同步后的本机时区。

### 修复

- OAuth 用户在认证中心已提供真实邮箱时,优先写入真实邮箱。
- userinfo 缺失邮箱时的可读占位邮箱回退更稳定。

### 测试

- 新增 OAuth 测试覆盖 `id_token` claims 合并。
- store 层 OAuth 邮箱回退测试保持通过。

## v1.0.0 (2026-05-03) — 时区 / OAuth / 提醒回归

### 修复

- 用户、管理员、todo、reminder 的默认时区统一为 `Asia/Shanghai`,统计接口异常回退也改为上海时区。
- OAuth 资料解析兼容 QiShu 可能返回的 `email_name` 等邮箱字段,避免邮箱显示为 provider subject 生成的长串。
- OAuth 用户再次登录时不再覆盖已手动修改的 `display_name`,只同步邮箱。

### 测试

- 新增 store 层测试覆盖默认时区、OAuth 邮箱同步、OAuth 显示名不覆盖。
- 新增 OAuth 解析测试覆盖 QiShu 风格邮箱字段。

## v0.3.0 (2026-04-29) — 阶段 6 + 阶段 11

### 新增

- **Web 完整管理端**(规格 §14 阶段 6,新目录 `web/`)
  - Vue 3 + Vite 6 + TypeScript + Pinia + vue-router 4,纯前端 SPA。
  - 路由表覆盖规格 §5 MVP 的全部任务视图:
    - 今日 / 明天 / 本周 / 过期 / 无日期 / 全部 / 已完成。
    - 清单分页(`/list/:id`)。
    - 日历视图(月度,可点格快速添加,事件按时间显示)。
    - 番茄专注(支持启动 / 完成 / 放弃,按用户时区计时,支持关联 todo)。
    - 数据复盘(SVG 自绘双柱图 + 番茄分钟柱图,无第三方图表库)。
    - 通知中心(列表 + 详情 + 投递日志,只看未读 / 全部已读)。
    - Telegram 绑定(deep-link + 一次性 token + 3 秒轮询;严禁前端输入 chat_id / 密码 / 验证码)。
    - 用户设置(浏览器通知权限请求,不承担本地强提醒)。
  - SSE 订阅(`/ws/events`)用 `fetch` + `ReadableStream` 自己读取(浏览器原生 `EventSource` 不能加 `Authorization` 头)。SSE 推到的通知会:
    - 自增未读计数。
    - 弹应用内 toast(6 秒后自动关闭)。
    - 触发 `Notification` API(若用户授权)。
    - 后台拉一次 `/api/notifications` 兜底。
  - 401 自动 refresh,只飞一次,所有并发请求等同一个 promise。Refresh 失败统一跳 `/login`。
  - 离线状态用 `navigator.onLine` 兜底,顶部警告条;按规格 §4 不允许离线写。
  - 全部 Pinia 状态在内存里,只 `localStorage` 持久化 token / user(无 sync queue)。
  - 主题色变量化,自动深色模式(`prefers-color-scheme: dark`)。

- **后端:番茄专注 + 数据复盘**(规格 §14 阶段 11,§10 / §11)
  - 数据库 migration v2:`pomodoro_sessions` 表(状态机 active → completed / abandoned,`actual_duration_seconds` 服务端计算 + clamp 到 [0, planned*4])。
  - `internal/store/pomodoro.go` —— 创建 / 列表(支持 todo / status / kind / from / to / 分页) / 改备注 / 完成 / 放弃 / 删除。`finalize` 用乐观并发("status='active'" WHERE 子句 + RowsAffected=0 → ErrConflict)。
  - `internal/store/stats.go` —— 4 个聚合查询。每天 / 每周按用户时区分桶(枚举日 + 单条 SQL,避免 SQLite 缺乏 TZ-aware GROUP BY)。
  - `internal/handlers/pomodoro.go` —— 7 个端点:
    - `GET    /api/pomodoro/sessions`(filter: todo_id / status / kind / from / to / limit / offset)
    - `POST   /api/pomodoro/sessions`
    - `GET    /api/pomodoro/sessions/{id}`
    - `PUT    /api/pomodoro/sessions/{id}`(只允许改 note)
    - `DELETE /api/pomodoro/sessions/{id}`
    - `POST   /api/pomodoro/sessions/{id}/complete`
    - `POST   /api/pomodoro/sessions/{id}/abandon`
  - `internal/handlers/stats.go` —— 4 个端点:
    - `GET /api/stats/summary`
    - `GET /api/stats/daily?from=YYYY-MM-DD&to=YYYY-MM-DD`
    - `GET /api/stats/weekly?from=YYYY-MM-DD&to=YYYY-MM-DD`
    - `GET /api/stats/pomodoro?from=YYYY-MM-DD&to=YYYY-MM-DD`(总数 + by_status + by_kind 秒 + 每日)
  - 区间日期是用户时区下的 YYYY-MM-DD,缺省 30 天;daily 上限 366 天,weekly 上限 53 周(防止滥用)。
  - `models.PomodoroSession` JSON DTO 加进 `internal/models/models.go`。
  - sync_events 新增 entity_type:`pomodoro`(create / update / delete 都会写)。

### 修改

- `cmd/server/main.go` 装配 `PomodoroStore` 与 `StatsStore`。
- `internal/server/server.go` 增加 `Pomos` / `Stats` 字段并注册全部新路由。
- 仓库根目录改为多包结构:`server/`(原 v0.2.0 包,go.mod 不变,只是从仓库根挪到 `server/`)+ `web/`(新增)。

### 已知限制(留给后续阶段)

- Android 原生与 Windows Tauri 客户端尚未实现,留作阶段 7-10。
- 调度器仍然是单进程单 goroutine,多副本部署需要外加去重机制。
- 子任务 / 番茄会话不会自动从 todo 列表的 sync_events 关联推送(客户端拉 sync 看到 `todo.updated` 时自行刷新即可)。
- Web 端图表用 SVG 自绘,只覆盖最常用的双柱 + 单柱,没有交互式 brush / zoom。

### 测试

- `go test ./...` 仍然只覆盖 rrule / telegram / events 三个无 DB 包。番茄专注 / stats 的 SQL 集成测试留给阶段 12 部署期 e2e。
- Web 端:
  - `npm run type-check`(等价于 `vue-tsc --noEmit`)通过零错误。
  - `npm run build` 产物 `dist/index-*.js` ~112 KB / gzip ~43 KB。

## v0.2.0 (2026-04-28) — 阶段 4 + 阶段 5

### 新增

- **Telegram Bot deep-link 绑定**(规格 §8)
  - `internal/telegram/client.go` —— 极简 Bot API client(`SendMessage` / `SetWebhook` / `DeleteWebhook`),纯标准库,15s 请求超时,1MB 响应上限。
  - `internal/telegram/bot.go` —— 解析 `/start bind_<token>` deep-link 负载。
  - `internal/store/telegram.go` —— `bind_token` 一次性签发 / 校验 / 消费(事务式 UPSERT 到 `telegram_bindings`),`telegram_bindings` 列表 / 解绑 / GC。
  - `internal/handlers/telegram.go` —— 6 个 HTTP 端点。
- **服务端调度器与通知投递**(规格 §3、§14 阶段 5)
  - `internal/scheduler/scheduler.go` —— 进程内单 goroutine,默认 5 秒 tick;扫描 `next_fire_at <= now` 的提醒。
  - `internal/store/notifications.go` —— `notifications` + `notification_deliveries` 读写。
- **SSE 实时推送**(规格 §3 `/ws`)
  - `internal/events/hub.go` —— 进程内事件总线,per-user 订阅,non-blocking publish。
  - `internal/handlers/sse.go` —— `GET /ws/events`,25 秒心跳。

## v0.1.0 — 阶段 1 + 2 + 3

后端骨架、TODO/列表/子任务、RRULE 提醒规则。详见 README。
