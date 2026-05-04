# TaskFlow

> 多用户 TODO + Android / Windows 强提醒 + 全端管理系统。
> 本仓库已完成规格文档 v2.2 的 **全部 12 个阶段**。

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| 1 | 后端骨架(Go + SQLite WAL + JWT) | ✅ |
| 2 | TODO / 列表 / 子任务 API | ✅ |
| 3 | RRULE 提醒规则 | ✅ |
| 4 | Telegram Bot deep-link 绑定与推送 | ✅ |
| 5 | 服务端调度器 / 通知分发 / SSE | ✅ |
| 6 | Web 完整管理端 | ✅ |
| 7 | Android 完整管理端(Kotlin / Compose) | ✅ **v1.4.0** |
| 8 | Android 强提醒(AlarmManager / 锁屏全屏 / 重启恢复) | ✅ **v1.4.0** |
| 9 | Windows 完整管理端(Tauri 复用 Web) | ✅ **v1.4.0** |
| 10 | Windows 强提醒(Toast / 总在最前窗 / 响铃) | ✅ **v1.4.0** |
| 11 | 番茄钟 / 统计 | ✅ |
| 12 | 部署套件(systemd / nginx / HTTPS / 备份) | ✅ **v1.4.0** |

---

## 仓库结构

```
taskflow/
├── server/                  # Go 后端 (Go 1.22+, SQLite WAL)
│   ├── cmd/server/main.go
│   ├── internal/            # store / handlers / middleware / scheduler / sse / telegram / pomodoro / stats
│   ├── config.example.toml
│   ├── go.mod
│   └── Makefile
├── web/                     # Vue 3 + Vite + Pinia + vue-router + TypeScript
│   ├── src/{api,types,utils,stores,components,views,tauri.ts}
│   └── package.json
├── windows/                 # Tauri v2 桌面客户端(复用 ../web 源码)
│   ├── src-tauri/{Cargo.toml,tauri.conf.json,src/*.rs}
│   ├── vite.config.ts       # root=../web,alias @ = ../web/src
│   └── package.json
├── android/                 # Android 原生(Kotlin 2.0 / Compose Material3)
│   ├── app/
│   │   ├── build.gradle.kts
│   │   └── src/main/{AndroidManifest.xml,res/,java/com/example/taskflow/...}
│   ├── gradle/libs.versions.toml
│   └── settings.gradle.kts
├── deploy/                  # 阶段 12 部署套件
│   ├── README.md            # 完整运维手册
│   ├── systemd/taskflow.service
│   ├── nginx/{taskflow.conf,taskflow.dev.conf}
│   ├── scripts/{install,backup,restore,telegram-setup,certbot-renew-hook}.sh
│   └── samples/config.production.toml
├── Makefile                 # 顶层编排:make server-build / web-build / windows-build / android-debug / dist
├── README.md                # 本文件
└── CHANGELOG.md
```

---

## 快速上手

### 本地开发(全端)

```bash
# 1. 启动后端
make server-run                 # http://localhost:8080
# 或: cd server && make run

# 2. 启动 Web 前端(开发模式 hot reload)
make web-dev                    # http://localhost:5173

# 3.(可选)Windows 桌面客户端
#   需要 Rust toolchain + WebView2 Runtime
make windows-dev

# 4.(可选)Android 客户端
#   在 Android Studio 中打开 android/ 目录
#   或:make android-debug 后 adb install
```

### 生产部署 (规格阶段 12)

详细步骤见 [`deploy/README.md`](deploy/README.md)。简略版:

```bash
# 在本地仓库根
make build-linux-amd64
scp server/taskflow-server-linux-amd64 user@vps:/tmp/
scp -r web/dist                          user@vps:/tmp/taskflow-web
scp -r deploy                            user@vps:/tmp/

# VPS 上一键
ssh user@vps
sudo /tmp/deploy/scripts/install.sh \
    --binary /tmp/taskflow-server-linux-amd64 \
    --web    /tmp/taskflow-web \
    --domain todo.example.com \
    --email  you@example.com
```

`install.sh` 会:创建系统用户 → 复制二进制 / Web → 写 systemd unit → 写 nginx → 申请 Let's Encrypt 证书 → 注册 cron 备份。

### Docker 部署(单机一键)

仓库根目录提供了 `docker-compose.yml`,把后端 + nginx + 静态前端打包成两个容器:

```bash
cp .env.example .env       # 至少修改 TASKFLOW_JWT_SECRET、ADMIN_PASSWORD
docker compose up -d --build
# 浏览器打开 http://localhost:8080
```

变量说明见 [`.env.example`](.env.example);要点:

- `TASKFLOW_JWT_SECRET`(必填,32 字符以上随机串):`openssl rand -hex 32`
- `ADMIN_EMAIL` / `ADMIN_PASSWORD`:首次启动时引导出一个管理员账号(默认 `admin@example.com` / `ChangeMeNow123!`)。**上线后请立即在管理面板里改密码,然后从 `.env` 中清空 `ADMIN_PASSWORD`。**

数据持久化在仓库目录下的 `./data`(SQLite)与 `./backup`(备份),容器删除后数据保留。

### 管理面板(管理员独占)

满足 `is_admin=true` 的账号会在左侧栏看到「管理面板」入口,提供:

- 系统状态:进程 / 内存 / 磁盘(数据库分区)/ 数据库大小与各表行数
- 用户管理:增删改、提升管理员、禁用/启用、按邮箱搜索
- 审计日志:所有管理员动作的可搜索时间线
- 数据清理:已完成任务、软删任务、旧通知、过期 token、`VACUUM`,均支持"试运行"
- 系统设置:当前生效配置摘要(只读)

服务端会写一份 `audit_logs` 表;首次启动时 `bootstrapAdmin()` 根据 `ADMIN_EMAIL/ADMIN_PASSWORD`
建管理员或把已存在用户提升为管理员(密码不会被覆盖)。


---

## 端定位(规格 §2)

| 端 | 完整管理 | 强提醒 | 备注 |
| --- | --- | --- | --- |
| **Web 浏览器** | ✅ | ❌ | 任何机器都能管理,但浏览器沙箱里没有真正的"强提醒"(无法响铃 / 全屏锁屏)。 |
| **Windows (Tauri)** | ✅(复用 Web) | ✅ | 系统 Toast + 总在最前 Alarm 窗 + rodio 响铃,Rust 进程后台调度,断网照触发。 |
| **Android (Kotlin)** | ✅ | ✅ | AlarmManager + 锁屏全屏 Activity + Foreground Service 响铃 + BootReceiver 重启恢复。 |
| **Telegram Bot** | ✅ 命令式查询 / 完成 | ✅ 推送 | `/start bind_<token>` 完成绑定,推送到点提醒。 |

---

## 离线策略(规格 §4)

- **服务端**永远是 source of truth。
- **Windows / Android** 客户端在断网时:
  - 已同步的数据可以**只读**展示;
  - 已注册到本地调度器(AlarmManager / Tauri scheduler)的 reminder **会照常触发**响铃 / 弹窗;
  - 但**不接受**新建 / 编辑(任何写入操作都会显示 "当前离线")。
- 用户在响铃窗口里点 "完成" 时:
  - 在线 → 调 `/api/todos/{id}/complete`;
  - 离线 → 仅停响铃 + 提示 "联网后请在主界面再次确认完成"。

这是为了保证**永不丢失**用户操作的语义:离线时永远不在本地把 todo 标成已完成,避免下次同步把服务端覆盖错。

---

## 安全

- 后端:JWT(access 15 min / refresh 30 d),bcrypt cost = 11,SQLite WAL,所有破坏性操作都校验所有权。
- 部署:systemd `NoNewPrivileges` / `ProtectSystem=strict` / 资源上限;nginx HSTS / CSP / `X-Frame-Options=DENY`;TLS 1.2+ via certbot。
- Windows:Tauri 默认开 `custom-protocol`,CSP 限定 `connect-src self ipc: https: http:`。
- Android:token 存 EncryptedSharedPreferences(AES-256-GCM, 主密钥在 Keystore);明文 HTTP 仅 debug 构建对 `10.0.2.2 / 127.0.0.1` 开放。
- Telegram webhook:`secret_token` HTTP header,后端 constant-time 比较。

---

## 文档

- `server/README.md` — 后端 API 一览 + Telegram bot 配置
- `web/README.md` — Web 前端开发 / 构建
- `windows/README.md` — Tauri 开发 / 打包 msi/nsis
- `android/README.md` — Android 构建 / 权限矩阵 / 强提醒流程图
- `deploy/README.md` — 完整运维手册
- `CHANGELOG.md` — 版本历史
