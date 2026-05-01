# taskflow-server

> 多用户 TODO + 强提醒系统的后端 — 阶段 1–5 + 阶段 11 实现。
> Go 1.22+ / SQLite (WAL) / 纯 Go 编译,适配低内存 VPS。

本仓库目前完成了规格文档 v2.2 的 **阶段 1**(后端骨架)、**阶段 2**(TODO / 列表 / 子任务)、**阶段 3**(基于 RRULE 的提醒规则)、**阶段 4**(Telegram Bot deep-link 绑定)、**阶段 5**(服务端调度器、通知投递、SSE 实时推送),以及 **阶段 11**(番茄专注会话 + 数据复盘聚合)。Android / Windows 客户端属于后续阶段,暂未实现。Web 端见仓库根目录 `../web/`。

---

## 目录结构

```
server/
├── cmd/server/         # 程序入口
├── internal/
│   ├── auth/           # JWT + bcrypt + refresh token
│   ├── config/         # TOML 配置加载
│   ├── db/             # SQLite 打开 + 迁移(schema v1 + v2)
│   ├── events/         # 进程内 SSE 事件总线(per-user 广播,non-blocking)
│   ├── handlers/       # HTTP 处理器(含 pomodoro / stats)
│   ├── middleware/     # 认证 / 日志 / 恢复 / CORS
│   ├── models/         # JSON DTO
│   ├── rrule/          # RFC 5545 RRULE next-fire 计算
│   ├── scheduler/      # 后台调度循环:扫描 due reminders → 投递 → 推进 next_fire_at
│   ├── server/         # 路由组装(Go 1.22 ServeMux)
│   ├── store/          # SQL 存取 + 同步事件(含 pomodoro / stats)
│   └── telegram/       # Bot API client(sendMessage/setWebhook)+ /start payload 解析
├── config.example.toml
├── go.mod
├── Makefile
└── README.md
```

---

## 快速开始

### 1. 准备配置

```bash
cp config.example.toml config.toml

# 生成强随机的 JWT 密钥
openssl rand -hex 32 | tee /tmp/jwt-secret
```

把生成的 64 位十六进制串填到 `config.toml` 的 `[auth].jwt_secret`。

### 2. 拉依赖并运行

```bash
go mod tidy
go run ./cmd/server -config config.toml
# 或:
make run
```

服务启动后会:

1. 在 `data/taskflow.db` 自动创建 SQLite 数据库(WAL 模式)。
2. 应用 schema v1(11 张业务表 + sync_events)。
3. 启动后台调度器(默认每 5 秒扫一次到期提醒)。
4. 监听 `127.0.0.1:8080`。

### 3. 编译

```bash
make build              # 当前平台
make build-linux-amd64  # 给 x86 VPS 用
make build-linux-arm64  # 给 ARM 服务器(如树莓派)用
```

产物是单个静态二进制,**不依赖 libc**(用了 `modernc.org/sqlite`,纯 Go 实现)。直接拷到目标机就能跑。

---

## 实现要点 / 与规格文档的偏差

| 主题 | 决策 |
| --- | --- |
| 路由 | Go 1.22 标准库 `http.ServeMux` 的方法路由,不引入第三方路由库 |
| SQLite 驱动 | `modernc.org/sqlite`(纯 Go),省掉 CGO,能交叉编译 |
| 时区 | **每条 todo 与每条 reminder 都带 timezone 字段**(规格文档漏写)。所有时间存 UTC,RRULE 在用户时区展开,正确处理 DST。 |
| RRULE | 使用 `teambition/rrule-go`,符合 RFC 5545。保存时 `ValidateRRule`,变更时立即 `ComputeNextFire`。 |
| 离线 | 服务端不接受离线写。Android/Windows 客户端只缓存"已同步的本地提醒"用于断网响铃,不做离线写入冲突合并(MVP 简化)。 |
| 同步 | 用 `sync_events` 增量事件流,客户端按 cursor 拉取(`/api/sync/pull?since=...`)。 |
| Refresh Token | 32 字节随机 + SHA-256 哈希存库,刷新即旋转。 |
| 软删除 | `lists / todos / reminder_rules` 都用 `deleted_at` 软删,便于客户端通过 `sync_events` 拉到删除事件。 |
| Telegram 绑定 | **只接受 Bot deep-link**(`tg://resolve` / `https://t.me/Bot?start=bind_xxx`),严禁前端传 chat_id 决定接收者(规格 §8)。 |
| Telegram webhook 校验 | 用 Telegram 标准的 `secret_token`(`X-Telegram-Bot-Api-Secret-Token` header,constant-time 比较),不暴露在 URL 中。 |
| 调度器 | 进程内单 goroutine,每 tick 扫 `next_fire_at <= now`。投递失败不阻塞推进 `next_fire_at`,从而追赶停机也只触发一次。 |
| SSE | `/ws/events` 是 Server-Sent Events(单向 server→client),不做 WebSocket;每 25s 心跳防代理切流。客户端断线重连后应先 sync pull 一遍兜底。 |

---

## API 参考

所有响应 / 请求体均为 JSON。时间统一 RFC 3339(UTC,例:`2026-04-28T10:23:45Z`)。
错误形状统一为:

```json
{ "error": { "code": "bad_request", "message": "title required" } }
```

需要认证的接口在 `Authorization: Bearer <access_token>` 头中带 access token。

### 健康检查

```
GET  /healthz
```

### 认证

```
POST /api/auth/register
POST /api/auth/login
POST /api/auth/refresh
POST /api/auth/logout      (auth)
GET  /api/auth/me          (auth)
```

注册请求体:

```json
{
  "email": "you@example.com",
  "password": "at-least-8-chars",
  "display_name": "Me",
  "timezone": "Asia/Shanghai",
  "device_id": "optional-uuid"
}
```

返回:

```json
{
  "access_token":  "...",
  "access_token_expires_at":  "2026-04-28T10:38:45Z",
  "refresh_token": "...",
  "refresh_token_expires_at": "2026-05-28T10:23:45Z",
  "user": { "id": 1, "email": "...", "timezone": "Asia/Shanghai", ... }
}
```

### 列表(Lists)

```
GET    /api/lists                   (auth)
POST   /api/lists                   (auth)
PUT    /api/lists/{id}              (auth)
DELETE /api/lists/{id}              (auth)
```

### TODO

```
GET    /api/todos                   (auth)
POST   /api/todos                   (auth)
GET    /api/todos/{id}              (auth)
PUT    /api/todos/{id}              (auth)
DELETE /api/todos/{id}              (auth)
POST   /api/todos/{id}/complete     (auth)
POST   /api/todos/{id}/uncomplete   (auth)
```

`GET /api/todos` 支持的查询参数:

| 参数 | 含义 |
| --- | --- |
| `filter` | `today` / `tomorrow` / `this_week` / `overdue` / `no_date` / `completed` / `all`(用用户时区判定) |
| `list_id` | 按列表过滤 |
| `search` | 标题/描述模糊匹配 |
| `limit`, `offset` | 分页 |
| `order_by` | `due_at_asc`(默认)/ `created_desc` / `priority_desc` / `sort_order` |
| `include_done` | `true` 时包含已完成 |

### 子任务(Subtasks)

```
GET    /api/todos/{todo_id}/subtasks      (auth)
POST   /api/todos/{todo_id}/subtasks      (auth)
PUT    /api/subtasks/{id}                 (auth)
DELETE /api/subtasks/{id}                 (auth)
POST   /api/subtasks/{id}/complete        (auth)
POST   /api/subtasks/{id}/uncomplete      (auth)
```

### 提醒(Reminders)

```
GET    /api/reminders                     (auth)
POST   /api/reminders                     (auth)
GET    /api/reminders/{id}                (auth)
PUT    /api/reminders/{id}                (auth)
DELETE /api/reminders/{id}                (auth)
POST   /api/reminders/{id}/enable         (auth)
POST   /api/reminders/{id}/disable        (auth)
```

请求体两种形态(必须二选一):

**单次提醒:**

```json
{
  "todo_id": 12,
  "title": "服药",
  "trigger_at": "2026-04-29T01:00:00Z",
  "timezone": "Asia/Shanghai",
  "channel_local": true,
  "channel_telegram": false
}
```

**周期提醒(每 6 个月一次的体检):**

```json
{
  "todo_id": 13,
  "title": "半年体检",
  "rrule": "FREQ=MONTHLY;INTERVAL=6",
  "dtstart": "2026-01-15T01:00:00Z",
  "timezone": "Asia/Shanghai",
  "channel_local": true,
  "channel_telegram": true
}
```

服务端会即时计算 `next_fire_at` 并随响应返回。

### Telegram(阶段 4)

```
POST /api/telegram/bind-token             (auth)  生成一次性 deep-link
GET  /api/telegram/bind-status?token=...  (auth)  轮询绑定状态
GET  /api/telegram/bindings               (auth)  列出当前用户的所有绑定
POST /api/telegram/unbind                 (auth)  body: {"id": <binding_id>}
POST /api/telegram/test                   (auth)  body: {"binding_id": <id>}
POST /api/telegram/webhook                (公开,但要带 secret token)
```

**绑定流程**(规格文档 §8 严格要求,前端**不允许**输入 chat_id / 密码 / 验证码):

```
[1] Web/Android/Windows 客户端按 "绑定 Telegram"
                ↓
[2] 调 POST /api/telegram/bind-token →
    返回 { token, expires_at, deep_link_web, deep_link_app }
                ↓
[3] 客户端拉起 Telegram:
        Web/Win:  打开 deep_link_web   (https://t.me/<bot>?start=bind_<token>)
        Android:  打开 deep_link_app  (tg://resolve?domain=<bot>&start=bind_<token>)
                ↓
[4] 用户在 Telegram 客户端按 [START]
                ↓
[5] Telegram 把 "/start bind_<token>" 推到 /api/telegram/webhook
    服务端校验 secret_token,提取 chat_id,UPSERT telegram_bindings
                ↓
[6] 客户端轮询 GET /api/telegram/bind-status?token=<token>
    -> {"status": "bound", "binding": {...}}
```

`/api/telegram/bind-token` 响应示例:

```json
{
  "token": "5e9c…",
  "expires_at": "2026-04-28T11:33:45Z",
  "bot_username": "TaskFlowBot",
  "deep_link_web": "https://t.me/TaskFlowBot?start=bind_5e9c…",
  "deep_link_app": "tg://resolve?domain=TaskFlowBot&start=bind_5e9c…"
}
```

**配置 webhook(部署时一次性)**:

```bash
TG_TOKEN="123456:abc..."     # bot_token
TG_SECRET="$(cat config.toml | grep webhook_secret | head -1 | cut -d'"' -f2)"
PUBLIC_URL="https://your.domain"

curl -s -X POST "https://api.telegram.org/bot$TG_TOKEN/setWebhook" \
  -H 'Content-Type: application/json' \
  -d "{
    \"url\": \"$PUBLIC_URL/api/telegram/webhook\",
    \"secret_token\": \"$TG_SECRET\",
    \"allowed_updates\": [\"message\"]
  }"
```

### 通知中心(阶段 5)

服务端调度器每次触发提醒都会写一行 `notifications`(给 UI 用)+ 一行或多行 `notification_deliveries`(投递审计)。

```
GET  /api/notifications?only_unread=&limit=&offset=   (auth)
GET  /api/notifications/unread-count                  (auth)
POST /api/notifications/read-all                      (auth)
GET  /api/notifications/{id}                          (auth)  含 deliveries 列表
POST /api/notifications/{id}/read                     (auth)
```

`GET /api/notifications` 响应:

```json
{
  "items": [
    {
      "id": 17,
      "user_id": 1,
      "reminder_rule_id": 4,
      "todo_id": 12,
      "title": "半年体检",
      "body": "到点啦 ⏰  时间:2026-04-28 19:00 CST",
      "fire_at": "2026-04-28T11:00:00Z",
      "is_read": false,
      "created_at": "2026-04-28T11:00:01Z"
    }
  ],
  "unread_count": 3
}
```

### 实时推送(阶段 5,SSE)

```
GET /ws/events    (auth)
```

`text/event-stream`,每个事件:

```
event: notification
data: {"type":"notification","notification_id":17,"reminder_rule_id":4,...}

```

每 25 秒会有一条 `: heartbeat` 注释,保持代理不切流。客户端示例:

```js
const es = new EventSource('/ws/events', { withCredentials: true });
// EventSource 不支持自定义 header,所以前端通常通过 cookie 或者 query token 鉴权
es.addEventListener('notification', (ev) => {
  const data = JSON.parse(ev.data);
  toast(data.title, data.body);
});
```

> 注意:浏览器的 `EventSource` 不能带 `Authorization: Bearer` header。生产部署可在 nginx 层把 access_token 从 `?token=` 转成 header,或在 Web 端用 `fetch` + `ReadableStream` 自己读 SSE。Android/Windows 客户端用 OkHttp / `reqwest` 的流式响应即可加 header。

### 番茄专注(阶段 11)

```
GET    /api/pomodoro/sessions                          (auth)  列表/过滤/分页
POST   /api/pomodoro/sessions                          (auth)  开始一个会话(状态 active)
GET    /api/pomodoro/sessions/{id}                     (auth)  详情
PUT    /api/pomodoro/sessions/{id}                     (auth)  只允许改 note
DELETE /api/pomodoro/sessions/{id}                     (auth)
POST   /api/pomodoro/sessions/{id}/complete            (auth)  active -> completed
POST   /api/pomodoro/sessions/{id}/abandon             (auth)  active -> abandoned
```

创建请求体:

```json
{
  "todo_id": 12,
  "planned_duration_seconds": 1500,
  "kind": "focus",          // focus | short_break | long_break,默认 focus
  "note": "复盘文档"
}
```

完成 / 放弃时,服务端在事务里用 `WHERE status = 'active'` 做乐观并发,保证不会因为前端双击而被多次终结。`actual_duration_seconds = clamp(now - started_at, 0, planned*4)`,防止时钟回拨 / 挂机异常值。

`GET /api/pomodoro/sessions` 支持的查询参数:

| 参数 | 含义 |
| --- | --- |
| `todo_id`        | 只看绑定到该 todo 的会话 |
| `status`         | `active` / `completed` / `abandoned` |
| `kind`           | `focus` / `short_break` / `long_break` |
| `from`, `to`     | RFC3339,按 `started_at` 闭开区间 |
| `limit`, `offset`| 分页(默认 100,最大 500) |

### 数据复盘(阶段 11)

```
GET /api/stats/summary                                 (auth)  今日/本周总览
GET /api/stats/daily?from=YYYY-MM-DD&to=YYYY-MM-DD     (auth)  每日明细
GET /api/stats/weekly?from=YYYY-MM-DD&to=YYYY-MM-DD    (auth)  每周明细
GET /api/stats/pomodoro?from=YYYY-MM-DD&to=YYYY-MM-DD  (auth)  番茄聚合 + 每日
```

- 区间日期是用户**时区**下的 `YYYY-MM-DD`,左闭右开。缺省 `[今天 - 30, 今天 + 1)`。
- `daily` 上限 366 天,`weekly` 上限 53 周,超过返回 400。
- 番茄统计只计 `kind = focus` 且 `status IN ('completed','abandoned')` 的实际秒数(`actual_duration_seconds`)。

`GET /api/stats/summary` 响应示例:

```json
{
  "todos_total": 42,
  "todos_open": 18,
  "todos_completed": 24,
  "todos_overdue": 3,
  "todos_due_today": 5,
  "completed_today": 4,
  "completed_this_week": 17,
  "pomodoro_today_seconds": 5400,
  "pomodoro_this_week_seconds": 21600
}
```

`GET /api/stats/pomodoro` 响应示例:

```json
{
  "from": "2026-04-01",
  "to":   "2026-04-30",
  "total_sessions": 56,
  "total_seconds": 84000,
  "by_status":  { "completed": 50, "abandoned": 6 },
  "by_kind_seconds": { "focus": 84000, "short_break": 0 },
  "daily": [
    { "date": "2026-04-01", "created": 3, "completed": 2, "pomodoro_seconds": 3000, "pomodoro_count": 2 }
  ]
}
```

### 增量同步

```
GET    /api/sync/cursor                   (auth)  # 返回当前 cursor
GET    /api/sync/pull?since=<cursor>&limit=500   (auth)
```

`/api/sync/pull` 返回:

```json
{
  "events": [
    { "id": 42, "entity_type": "todo", "entity_id": 7, "action": "updated", "created_at": "..." }
  ],
  "next_cursor": 42,
  "has_more": false
}
```

`entity_type` 取值:`todo` / `list` / `subtask` / `reminder` / `notification` / `telegram_binding` / `pomodoro`。

---

## curl 速查

```bash
# 注册
curl -s -X POST http://127.0.0.1:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@test.local","password":"hunter2hunter2","timezone":"Asia/Shanghai"}'

# 登录(把 access_token 存 $T)
T=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@test.local","password":"hunter2hunter2"}' \
  | jq -r .access_token)

# 创建 todo
curl -s -X POST http://127.0.0.1:8080/api/todos \
  -H "Authorization: Bearer $T" \
  -H 'Content-Type: application/json' \
  -d '{"title":"写后端","priority":2,"due_at":"2026-04-30T01:00:00Z"}'

# 今天的 todo
curl -s -H "Authorization: Bearer $T" \
  'http://127.0.0.1:8080/api/todos?filter=today'

# 创建一条每 6 个月的体检提醒,服务端调度器会到点投递
curl -s -X POST http://127.0.0.1:8080/api/reminders \
  -H "Authorization: Bearer $T" \
  -H 'Content-Type: application/json' \
  -d '{
    "title":"半年体检",
    "rrule":"FREQ=MONTHLY;INTERVAL=6",
    "dtstart":"2026-01-15T01:00:00Z",
    "timezone":"Asia/Shanghai",
    "channel_local":true,
    "channel_telegram":true
  }'

# 拉一个 Telegram bind-token,然后从浏览器/手机点 deep_link_web
curl -s -X POST http://127.0.0.1:8080/api/telegram/bind-token \
  -H "Authorization: Bearer $T" | jq .

# 绑定完后查询绑定
curl -s -H "Authorization: Bearer $T" http://127.0.0.1:8080/api/telegram/bindings

# 给某 binding 发一条测试消息
curl -s -X POST http://127.0.0.1:8080/api/telegram/test \
  -H "Authorization: Bearer $T" \
  -H 'Content-Type: application/json' \
  -d '{"binding_id":1}'

# 读未读数
curl -s -H "Authorization: Bearer $T" \
  http://127.0.0.1:8080/api/notifications/unread-count

# 开始一个 25 分钟番茄
curl -s -X POST http://127.0.0.1:8080/api/pomodoro/sessions \
  -H "Authorization: Bearer $T" \
  -H 'Content-Type: application/json' \
  -d '{"planned_duration_seconds":1500,"kind":"focus","note":"复盘 v0.3.0"}'

# 完成一个会话(假设 id=1)
curl -s -X POST http://127.0.0.1:8080/api/pomodoro/sessions/1/complete \
  -H "Authorization: Bearer $T"

# 总览统计
curl -s -H "Authorization: Bearer $T" http://127.0.0.1:8080/api/stats/summary

# 最近 14 天每日明细(用户时区)
curl -s -H "Authorization: Bearer $T" \
  'http://127.0.0.1:8080/api/stats/daily?from=2026-04-15&to=2026-04-30'

# 订阅 SSE(curl 的 -N 关闭缓冲)
curl -N -H "Authorization: Bearer $T" http://127.0.0.1:8080/ws/events
```

---

## 部署速记

### systemd

```ini
# /etc/systemd/system/taskflow.service
[Unit]
Description=taskflow-server
After=network.target

[Service]
Type=simple
User=taskflow
WorkingDirectory=/opt/taskflow
ExecStart=/opt/taskflow/taskflow-server -config /opt/taskflow/config.toml
Restart=on-failure
RestartSec=5

# 安全加固
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/opt/taskflow/data
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd --system --home /opt/taskflow --shell /usr/sbin/nologin taskflow
sudo mkdir -p /opt/taskflow/data
sudo cp taskflow-server-linux-amd64 /opt/taskflow/taskflow-server
sudo cp config.example.toml /opt/taskflow/config.toml   # 编辑它,填 jwt_secret / telegram
sudo chown -R taskflow:taskflow /opt/taskflow
sudo systemctl daemon-reload
sudo systemctl enable --now taskflow
sudo journalctl -fu taskflow
```

### nginx 反向代理

把 `listen` 留在 `127.0.0.1:8080`,前面挂 nginx,加 HTTPS:

```nginx
server {
  listen 443 ssl http2;
  server_name todo.example.com;
  ssl_certificate     /etc/letsencrypt/live/todo.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/todo.example.com/privkey.pem;

  client_max_body_size 1m;

  # 普通 API
  location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 60s;
  }

  # SSE 长连接 —— 关掉 buffering,允许长 read timeout
  location /ws/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 1h;
  }

  # 健康检查
  location = /healthz {
    proxy_pass http://127.0.0.1:8080;
  }
}
```

### 备份

SQLite 在 WAL 模式下要么用 `sqlite3 taskflow.db ".backup '/backup/taskflow-$(date +%F).db'"`,要么:

```bash
sqlite3 /opt/taskflow/data/taskflow.db "VACUUM INTO '/backup/taskflow.db'"
```

**不要**直接 `cp taskflow.db`(没拷到 `-wal` 文件会拿到不完整数据)。

---

## 测试

```bash
go test ./...
```

当前覆盖:

| 包 | 覆盖 |
| --- | --- |
| `internal/rrule` | 单次 / 月+6 间隔 / 跨夏令时 / 非法规则 |
| `internal/telegram` | `ParseStartCommand` 多种形态、`ExtractBindToken`、Bot API client 成功/错误/disabled(httptest) |
| `internal/events` | Hub 单订阅、跨用户隔离、缓冲满非阻塞、Close 幂等、并发、Shutdown 关闭所有订阅 |

集成测试(数据库 / scheduler 全链路 / SSE 端到端)留给阶段 6 的客户端 e2e 一并补。

---

## 后续阶段

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| 1 | 后端骨架 | ✅ |
| 2 | TODO / 列表 / 子任务 API | ✅ |
| 3 | RRULE 提醒规则 | ✅ |
| 4 | Telegram Bot deep-link 绑定与推送 | ✅ |
| 5 | 服务端调度器 / 通知分发 / SSE | ✅ |
| 6 | Web 前端(Vue 3 + Vite + TS) | ✅(见仓库 `../web/`) |
| 7–8 | Android Kotlin + AlarmManager 强提醒 | ⏳ |
| 9–10 | Windows Tauri 客户端 + 本地通知 | ⏳ |
| 11 | 番茄钟 / 统计 | ✅ |
| 12 | 部署套件(nginx/systemd/HTTPS/备份) | 📝 README 已含示例 |
