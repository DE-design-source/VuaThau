#!/usr/bin/env bash
# Sao lưu Vua Thầu nghiệm thu: CSDL (pg_dump, giữ 14 bản) + ảnh/biên lai (bản sao đồng bộ).
# Chạy mỗi đêm bởi /etc/cron.d/vtnt-backup. Khôi phục CSDL:
#   docker exec -i vtnt-db pg_restore -U vuathau -d vuathau --clean --if-exists < /opt/vuathau-nt-backups/db/vtnt-YYYYMMDD-HHMMSS.dump
set -euo pipefail
DEST=/opt/vuathau-nt-backups
KEEP=14
mkdir -p "$DEST/db" "$DEST/data"
chmod 700 "$DEST"

ts=$(date +%Y%m%d-%H%M%S)
docker exec vtnt-db pg_dump -U vuathau -d vuathau -Fc > "$DEST/db/vtnt-$ts.dump.tmp"
mv "$DEST/db/vtnt-$ts.dump.tmp" "$DEST/db/vtnt-$ts.dump"
chmod 600 "$DEST/db/vtnt-$ts.dump"
ls -1t "$DEST"/db/vtnt-*.dump | tail -n +$((KEEP + 1)) | xargs -r rm -f

src=$(docker volume inspect vtnt_data -f '{{.Mountpoint}}')
rsync -a --delete "$src/" "$DEST/data/"
echo "$(date -Is) ok db=vtnt-$ts.dump data=$(du -sh "$DEST/data" | cut -f1)"
