#!/usr/bin/env bash
# telegram-setup.sh — 一次性把 Telegram webhook 注册到这台 VPS。
#
# 前提:已经在 config.toml 里填好 bot_token / bot_username / webhook_secret 并 restart 过 taskflow。
#
# 用法:
#   sudo ./telegram-setup.sh \
#       --bot-token "1234567:abcdef…" \
#       --secret "$(grep webhook_secret /opt/taskflow/config.toml | cut -d'\"' -f2)" \
#       --domain todo.example.com
#
# 子命令:
#   set       注册 webhook(默认)
#   delete    删除 webhook
#   info      查看当前 webhook 状态

set -euo pipefail

BOT_TOKEN=""
SECRET=""
DOMAIN=""
ACTION="set"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --bot-token) BOT_TOKEN="$2"; shift 2 ;;
        --secret)    SECRET="$2";    shift 2 ;;
        --domain)    DOMAIN="$2";    shift 2 ;;
        set|delete|info) ACTION="$1"; shift ;;
        -h|--help)
            sed -n '/^# 用法/,/^# 子命令/p' "$0"
            exit 0 ;;
        *) echo "unknown: $1" >&2; exit 1 ;;
    esac
done

[[ -n "$BOT_TOKEN" ]] || { echo "--bot-token 必填"; exit 1; }

case "$ACTION" in
    set)
        [[ -n "$DOMAIN" && -n "$SECRET" ]] || { echo "set: --domain --secret 必填"; exit 1; }
        URL="https://${DOMAIN}/api/telegram/webhook"
        echo "==> setWebhook $URL"
        curl -fsS -X POST "https://api.telegram.org/bot${BOT_TOKEN}/setWebhook" \
            -H 'Content-Type: application/json' \
            -d "$(cat <<JSON
{
  "url": "${URL}",
  "secret_token": "${SECRET}",
  "allowed_updates": ["message"],
  "drop_pending_updates": true
}
JSON
)" \
            | jq . 2>/dev/null || true
        echo
        ;;
    delete)
        echo "==> deleteWebhook"
        curl -fsS -X POST "https://api.telegram.org/bot${BOT_TOKEN}/deleteWebhook" \
            -H 'Content-Type: application/json' \
            -d '{"drop_pending_updates": true}' \
            | jq . 2>/dev/null || true
        ;;
    info)
        echo "==> getWebhookInfo"
        curl -fsS "https://api.telegram.org/bot${BOT_TOKEN}/getWebhookInfo" | jq . 2>/dev/null || true
        ;;
esac
