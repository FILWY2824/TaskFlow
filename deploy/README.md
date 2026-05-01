# 部署套件 (规格 v2.2 阶段 12)

> 这一目录把 server/ + web/dist 部署到一台小 VPS 的全部配置打包好。
> 目标:Ubuntu 22.04+ / Debian 12+,Nginx 反代,systemd 托管,HTTPS 走 Let's Encrypt,SQLite WAL 定时备份。

## 目录结构

```
deploy/
├── README.md                       # 本文件
├── nginx/
│   ├── todoalarm.conf              # 生产 HTTPS 反代(最终配置)
│   └── todoalarm.dev.conf          # 仅 HTTP,本地测试用
├── systemd/
│   └── todoalarm.service           # 后端 systemd unit
├── scripts/
│   ├── install.sh                  # 一键部署:复制文件、创建用户、注册 systemd
│   ├── backup.sh                   # SQLite VACUUM INTO 备份
│   ├── restore.sh                  # 备份还原(交互式确认)
│   ├── telegram-setup.sh           # 注册 Telegram webhook
│   └── certbot-renew-hook.sh       # certbot 续签后 reload nginx
└── samples/
    └── config.production.toml      # 生产配置模板(JWT secret 占位)
```

---

## 完整流程

### 1. 系统准备(VPS 新装机一次性)

```bash
sudo apt update && sudo apt install -y nginx certbot python3-certbot-nginx sqlite3
```

### 2. 编译产物上传

在本地仓库根目录:

```bash
# 一次性编译后端 + 前端
make build-linux-amd64                          # 适用 x86 VPS
# 或:
make build-linux-arm64                          # 适用 ARM(树莓派等)

# 上传
scp server/taskflow-server-linux-amd64 user@vps:/tmp/
scp -r web/dist                          user@vps:/tmp/todoalarm-web
scp -r deploy                            user@vps:/tmp/
```

### 3. 在 VPS 上一键安装

```bash
ssh user@vps
cd /tmp/deploy
sudo ./scripts/install.sh \
    --binary /tmp/taskflow-server-linux-amd64 \
    --web /tmp/todoalarm-web \
    --domain todo.example.com \
    --email you@example.com
```

`install.sh` 会:

1. 创建系统用户 `todoalarm`(无 home,无 shell)
2. 把二进制丢到 `/opt/todoalarm/taskflow-server`,`web/dist` 丢到 `/var/www/todoalarm`
3. 写 `/opt/todoalarm/config.toml`(随机生成 JWT secret;Telegram 配置留空)
4. 安装 systemd unit 并 enable + start
5. 写 nginx 配置并 reload
6. `certbot --nginx -d todo.example.com -m you@example.com` 申请证书
7. 注册 certbot deploy hook,续签后自动 reload

### 4. (可选) Telegram webhook

在你拿到 BotFather 的 token 之后:

```bash
sudo nano /opt/todoalarm/config.toml          # 填 bot_token / bot_username / webhook_secret
sudo systemctl restart todoalarm

sudo /tmp/deploy/scripts/telegram-setup.sh \
    --bot-token "1234:abcde…" \
    --secret "$(grep webhook_secret /opt/todoalarm/config.toml | cut -d'"' -f2)" \
    --domain todo.example.com
```

### 5. 备份

`install.sh` 会注册一个 `cron` 任务,每天凌晨 3 点跑 `backup.sh`,保留最近 14 天:

```cron
0 3 * * * /opt/todoalarm/backup.sh >> /var/log/todoalarm-backup.log 2>&1
```

也可以手动跑:

```bash
sudo /opt/todoalarm/backup.sh
```

备份输出在 `/opt/todoalarm/backup/todoalarm-YYYYMMDD-HHMMSS.db`。**不**直接 `cp .db`,而是用 `sqlite3 ... 'VACUUM INTO ...'`,正确处理 WAL。

---

## 卸载

```bash
sudo systemctl disable --now todoalarm
sudo rm /etc/systemd/system/todoalarm.service
sudo rm /etc/nginx/sites-enabled/todoalarm /etc/nginx/sites-available/todoalarm
sudo systemctl reload nginx
sudo userdel todoalarm

# 数据(谨慎!)
sudo rm -rf /opt/todoalarm /var/www/todoalarm
```

---

## 安全说明

- systemd unit 启用了 `NoNewPrivileges` / `PrivateTmp` / `ProtectSystem=strict` / `ProtectHome` / `ReadWritePaths` 限制,二进制只能写 `data/` 与 `backup/`。
- nginx 配置打开了 HSTS、`X-Frame-Options=DENY`、`Referrer-Policy=strict-origin-when-cross-origin`、`Content-Security-Policy`(默认只允许同源 + CDN 字体)。
- Telegram webhook 用 `secret_token` HTTP header 校验,服务端做 constant-time 比较。
- JWT 密钥由 `install.sh` 用 `openssl rand -hex 32` 生成,落盘前 `chmod 600`。
