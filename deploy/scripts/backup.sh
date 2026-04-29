#!/usr/bin/env bash
# backup.sh — 用 SQLite VACUUM INTO 做一次原子快照,保留最近 N 天。
#
# WAL 模式下不能直接 cp .db,会拿到不完整数据。VACUUM INTO 会写一个全新的、
# 自洽的数据库文件,期间对原库只持读锁,生产可在线跑。
#
# 由 cron 每天 03:00 调用,也支持手动跑。

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/todoalarm}"
DB="${INSTALL_DIR}/data/todoalarm.db"
BACKUP_DIR="${INSTALL_DIR}/backup"
KEEP_DAYS="${KEEP_DAYS:-14}"

if [[ ! -f "$DB" ]]; then
    echo "$(date -Iseconds) [WARN] db 不存在: $DB"
    exit 0
fi

mkdir -p "$BACKUP_DIR"

STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$BACKUP_DIR/todoalarm-$STAMP.db"

# VACUUM INTO 会把数据全量重写到新文件(自带索引、压缩);失败时清理临时文件。
trap 'rm -f "$OUT"' ERR

sqlite3 "$DB" "VACUUM INTO '$OUT'"

# 完整性校验(快速)
RES="$(sqlite3 "$OUT" 'PRAGMA integrity_check' | head -1)"
if [[ "$RES" != "ok" ]]; then
    echo "$(date -Iseconds) [ERROR] integrity_check 失败:$RES"
    rm -f "$OUT"
    exit 1
fi

# 压缩
gzip -9 "$OUT"
OUT_GZ="$OUT.gz"

# 清理过期备份
find "$BACKUP_DIR" -maxdepth 1 -type f -name 'todoalarm-*.db.gz' -mtime "+${KEEP_DAYS}" -delete 2>/dev/null || true

SIZE="$(stat -c %s "$OUT_GZ")"
echo "$(date -Iseconds) [OK] 备份完成 $OUT_GZ ($((SIZE / 1024)) KB)"
