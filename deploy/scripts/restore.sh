#!/usr/bin/env bash
# restore.sh — 把一份备份还原成当前数据库。会先停服务、备份原库、再覆盖。
#
# 用法:
#   sudo ./restore.sh /opt/taskflow/backup/taskflow-20260428-030000.db.gz

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/taskflow}"
DB="${INSTALL_DIR}/data/taskflow.db"

[[ "$EUID" -eq 0 ]] || { echo "需要 sudo / root"; exit 1; }
[[ $# -eq 1 ]] || { echo "用法: $0 <backup-file.db|.db.gz>"; exit 1; }

SRC="$1"
[[ -f "$SRC" ]] || { echo "找不到备份文件: $SRC"; exit 1; }

echo "WARN: 这会用 $SRC 覆盖现有数据库 $DB"
echo "  原库会先备份到 ${DB}.before-restore-$(date +%s)"
read -rp "确认还原? (yes/N) " ANS
[[ "$ANS" == "yes" ]] || { echo "取消"; exit 0; }

systemctl stop taskflow
echo "==> 服务已停"

if [[ -f "$DB" ]]; then
    BAK="${DB}.before-restore-$(date +%s)"
    cp -a "$DB" "$BAK"
    [[ -f "${DB}-wal" ]] && cp -a "${DB}-wal" "${BAK}-wal" || true
    [[ -f "${DB}-shm" ]] && cp -a "${DB}-shm" "${BAK}-shm" || true
    echo "==> 原库已备份到 $BAK"
fi

rm -f "$DB" "${DB}-wal" "${DB}-shm"

if [[ "$SRC" == *.gz ]]; then
    zcat "$SRC" > "$DB"
else
    cp "$SRC" "$DB"
fi
chown taskflow:taskflow "$DB"
chmod 644 "$DB"

# 完整性校验
RES="$(sqlite3 "$DB" 'PRAGMA integrity_check' | head -1)"
if [[ "$RES" != "ok" ]]; then
    echo "ERROR: integrity_check 失败: $RES"
    exit 2
fi
echo "==> integrity_check 通过"

systemctl start taskflow
sleep 2
systemctl is-active --quiet taskflow && echo "==> 服务已重启 ✓" || { echo "服务启动失败"; exit 3; }
