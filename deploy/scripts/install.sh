#!/usr/bin/env bash
# install.sh — 一键把 todoalarm 部署到这台 Ubuntu / Debian 机器上。
#
# 用法:
#   sudo ./install.sh \
#       --binary /tmp/todoalarm-server-linux-amd64 \
#       --web    /tmp/todoalarm-web \
#       --domain todo.example.com \
#       --email  you@example.com
#
# 选项:
#   --no-tls              跳过 certbot,使用 todoalarm.dev.conf(HTTP only)
#   --no-cron             跳过备份 cron 注册
#   --listen 127.0.0.1:8080  后端监听地址(默认本回环 8080)
#
# 退出码:0=成功,1=参数错误,2=系统检查失败,3=运行中报错。

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

INSTALL_DIR=/opt/todoalarm
WEB_DIR=/var/www/todoalarm
USER_NAME=todoalarm
LISTEN="127.0.0.1:8080"
NO_TLS=0
NO_CRON=0
BINARY=""
WEB=""
DOMAIN=""
EMAIL=""

err() { echo "ERROR: $*" >&2; exit "${2:-3}"; }
log() { echo "==> $*"; }

# 解析参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        --binary) BINARY="$2"; shift 2 ;;
        --web)    WEB="$2";    shift 2 ;;
        --domain) DOMAIN="$2"; shift 2 ;;
        --email)  EMAIL="$2";  shift 2 ;;
        --listen) LISTEN="$2"; shift 2 ;;
        --no-tls) NO_TLS=1;    shift ;;
        --no-cron) NO_CRON=1;  shift ;;
        -h|--help)
            sed -n '/^# 用法/,/^# 退出码/p' "$0"
            exit 0 ;;
        *) err "unknown arg: $1" 1 ;;
    esac
done

[[ "$EUID" -eq 0 ]] || err "必须用 sudo / root 跑(需要写 /opt /etc/systemd /etc/nginx)" 1
[[ -n "$BINARY" && -f "$BINARY" ]] || err "--binary 文件不存在: $BINARY" 1
[[ -n "$WEB" && -d "$WEB" && -f "$WEB/index.html" ]] || err "--web 目录无效(找不到 index.html): $WEB" 1
[[ -n "$DOMAIN" ]] || err "--domain 必填" 1
if [[ "$NO_TLS" -eq 0 ]]; then
    [[ -n "$EMAIL" ]] || err "--email 必填(certbot 需要,或加 --no-tls 跳过)" 1
fi

command -v nginx     >/dev/null 2>&1 || err "nginx 未安装。请先 'apt install -y nginx'" 2
command -v sqlite3   >/dev/null 2>&1 || err "sqlite3 未安装。请先 'apt install -y sqlite3'" 2
if [[ "$NO_TLS" -eq 0 ]]; then
    command -v certbot >/dev/null 2>&1 || err "certbot 未安装。请先 'apt install -y certbot python3-certbot-nginx'" 2
fi

# === 1. 创建系统用户 ===
log "确保系统用户 $USER_NAME 存在"
if ! id -u "$USER_NAME" >/dev/null 2>&1; then
    useradd --system --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin --no-create-home "$USER_NAME"
fi

# === 2. 目录骨架 ===
log "创建 $INSTALL_DIR 与 $WEB_DIR"
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/backup" "$WEB_DIR"

# === 3. 安装二进制 ===
log "安装二进制到 $INSTALL_DIR/todoalarm-server"
install -m 0755 "$BINARY" "$INSTALL_DIR/todoalarm-server"

# === 4. 部署前端 ===
log "复制 web 静态资源到 $WEB_DIR"
rsync -a --delete "$WEB/" "$WEB_DIR/"

# === 5. 配置文件(如果不存在则从模板生成,保留已存在的) ===
CONF="$INSTALL_DIR/config.toml"
if [[ -f "$CONF" ]]; then
    log "发现已有 $CONF,保留不动"
else
    log "首次部署,生成 $CONF(随机 JWT secret;Telegram 配置留空)"
    JWT="$(openssl rand -hex 32)"
    cat > "$CONF" <<EOF
[server]
listen = "${LISTEN}"
shutdown_timeout_seconds = 15
write_timeout_seconds = 30
read_timeout_seconds = 30

[database]
path = "${INSTALL_DIR}/data/todoalarm.db"

[auth]
jwt_secret = "${JWT}"
access_ttl_seconds = 900
refresh_ttl_seconds = 2592000
bcrypt_cost = 11

[log]
level = "info"

[telegram]
# 拿到 BotFather 的 token 后再填,否则全部 telegram 功能关闭。
bot_token = ""
bot_username = ""
# openssl rand -hex 32
webhook_secret = ""
bind_token_ttl_seconds = 600

[scheduler]
tick_interval_seconds = 5
batch_size = 200
disabled = false
EOF
    chmod 600 "$CONF"
fi

# === 6. 备份脚本 ===
log "安装 backup.sh 到 $INSTALL_DIR/backup.sh"
install -m 0755 "$SCRIPT_DIR/backup.sh" "$INSTALL_DIR/backup.sh"

# === 7. 权限 ===
chown -R "$USER_NAME:$USER_NAME" "$INSTALL_DIR"
chown -R www-data:www-data "$WEB_DIR"

# === 8. systemd unit ===
log "安装 systemd unit"
install -m 0644 "$DEPLOY_DIR/systemd/todoalarm.service" /etc/systemd/system/todoalarm.service
systemctl daemon-reload
systemctl enable todoalarm
systemctl restart todoalarm
sleep 2
if ! systemctl is-active --quiet todoalarm; then
    log "todoalarm.service 启动失败,最近日志:"
    journalctl -u todoalarm -n 30 --no-pager || true
    err "请排查后重试" 3
fi

# === 9. nginx ===
log "安装 nginx 配置"
if [[ "$NO_TLS" -eq 1 ]]; then
    sed "s/__DOMAIN__/$DOMAIN/g; s/_;/$DOMAIN;/g" \
        "$DEPLOY_DIR/nginx/todoalarm.dev.conf" \
        > /etc/nginx/sites-available/todoalarm
else
    sed "s/__DOMAIN__/$DOMAIN/g" \
        "$DEPLOY_DIR/nginx/todoalarm.conf" \
        > /etc/nginx/sites-available/todoalarm
fi
ln -sf /etc/nginx/sites-available/todoalarm /etc/nginx/sites-enabled/todoalarm
# 第一次部署时,certbot 需要先有一个 HTTP server 处理 challenge
# 上面 generated config 里已经包含 http -> https 的 server,且 listen 80,certbot 可注入。
nginx -t
systemctl reload nginx

# === 10. TLS via certbot ===
if [[ "$NO_TLS" -eq 0 ]]; then
    log "申请 / 续签 Let's Encrypt 证书 ($DOMAIN)"
    certbot --nginx --non-interactive --agree-tos \
        -m "$EMAIL" \
        -d "$DOMAIN" \
        --redirect

    # 安装 deploy hook(续签后 reload nginx)
    install -m 0755 "$SCRIPT_DIR/certbot-renew-hook.sh" /etc/letsencrypt/renewal-hooks/deploy/todoalarm.sh
fi

# === 11. cron 备份 ===
if [[ "$NO_CRON" -eq 0 ]]; then
    CRON="0 3 * * * $USER_NAME $INSTALL_DIR/backup.sh >> /var/log/todoalarm-backup.log 2>&1"
    if ! grep -qF "$INSTALL_DIR/backup.sh" /etc/crontab; then
        log "注册 cron(每天 03:00 备份,保留 14 天)"
        echo "$CRON" >> /etc/crontab
    else
        log "cron 已存在,跳过"
    fi
    touch /var/log/todoalarm-backup.log
    chown "$USER_NAME":"$USER_NAME" /var/log/todoalarm-backup.log
fi

# === 12. 健康自检 ===
log "等待后端就绪…"
for i in 1 2 3 4 5; do
    if curl -sf "http://${LISTEN}/healthz" > /dev/null; then
        log "后端 /healthz 通过 ✓"
        break
    fi
    sleep 1
done

cat <<EOF

==============================================================
✓ 部署完成

  二进制:        $INSTALL_DIR/todoalarm-server
  配置:          $INSTALL_DIR/config.toml  (chmod 600)
  数据库:        $INSTALL_DIR/data/todoalarm.db
  备份目录:      $INSTALL_DIR/backup/
  Web 静态:      $WEB_DIR
  systemd:       systemctl status todoalarm
  日志:          journalctl -fu todoalarm

  访问:
EOF
if [[ "$NO_TLS" -eq 1 ]]; then
    echo "    http://$DOMAIN/"
else
    echo "    https://$DOMAIN/"
fi
echo
echo "  下一步(可选):配 Telegram"
echo "    1) sudo nano $INSTALL_DIR/config.toml"
echo "    2) sudo systemctl restart todoalarm"
echo "    3) sudo $SCRIPT_DIR/telegram-setup.sh --bot-token <T> --secret <S> --domain $DOMAIN"
echo "=============================================================="
