#!/usr/bin/env bash
# certbot 续签 deploy hook —— 续签成功后 reload nginx 让新证书生效。
# 由 install.sh 安装到 /etc/letsencrypt/renewal-hooks/deploy/todoalarm.sh
set -e
systemctl reload nginx
