#!/usr/bin/env bash
# Chuyển nghiemthu.vuathau.com.vn từ bản Python cũ (nghiemthu-web) sang backend Go (vtnt-api).
#   1. sao lưu Caddyfile + toàn bộ dữ liệu cũ
#   2. dừng bản cũ (không xoá), chuyển dữ liệu sang Postgres mới
#   3. trỏ tên miền sang vtnt-api, reload Caddy
# Quay lại bản cũ: xem rollback.txt trong thư mục sao lưu in ra ở cuối.
set -euo pipefail
TS=$(date +%Y%m%d-%H%M%S)
BK=/opt/vuathau-nt-backups/cutover-$TS
CADDY=/opt/qspro/infra/deploy/Caddyfile
OLDVOL=$(docker volume inspect nghiemthu_data -f '{{.Mountpoint}}')
mkdir -p "$BK"
chmod 700 "$BK"

echo "== 1. sao lưu"
cp -a "$CADDY" "$BK/Caddyfile.before"
docker exec -i nghiemthu-web python - > "$BK/counts-old.txt" <<'EOF'
import sqlite3
c = sqlite3.connect('/data/nghiemthu.db')
for t in ['users', 'houses', 'photos', 'batches', 'log', 'adjust']:
    print(t, c.execute('select count(*) from ' + t).fetchone()[0])
EOF
cat "$BK/counts-old.txt"

echo "== 2. dừng bản cũ, chép dữ liệu"
docker stop nghiemthu-web >/dev/null
restore_old() {
  echo "!! lỗi: bật lại bản cũ"
  docker start nghiemthu-web >/dev/null || true
}
trap restore_old ERR
cp -a "$OLDVOL" "$BK/legacy"
du -sh "$BK/legacy"

echo "== 3. chuyển dữ liệu sang Postgres mới"
docker run --rm --network vtnt --env-file /opt/vuathau-nt/.env -e DATA_DIR=/data \
  -v vtnt_data:/data -v "$BK/legacy":/old --entrypoint /app/ntctl vtnt-api:latest \
  import-legacy -db /old/nghiemthu.db -src /old | tee "$BK/import.txt"
docker exec vtnt-db psql -U vuathau -d vuathau -Atc "SELECT 'users', count(*) FROM users UNION ALL SELECT 'houses', count(*) FROM houses
  UNION ALL SELECT 'photos', count(*) FROM photos UNION ALL SELECT 'batches', count(*) FROM batches
  UNION ALL SELECT 'logs', count(*) FROM house_logs UNION ALL SELECT 'adjust', count(*) FROM adjustments" | tee "$BK/counts-new.txt"

echo "== 4. trỏ tên miền sang vtnt-api"
[ "$(grep -c 'reverse_proxy nghiemthu-web:8000' "$CADDY")" = 1 ] || { echo "không thấy đúng 1 dòng reverse_proxy nghiemthu-web:8000"; false; }
sed -e 's#reverse_proxy nghiemthu-web:8000#reverse_proxy vtnt-api:8080#' \
    -e 's#^\# Nghiệm thu ảnh CTV: repo DE-design-source/vuathau-nghiemthu.*#\# Nghiệm thu ảnh CTV: backend Go, repo DE-design-source/VuaThau, chạy ở /opt/vuathau-nt (bản Python cũ: /opt/vuathau-nghiemthu, đã dừng)#' \
    "$CADDY" > "$BK/Caddyfile.new"
docker cp "$BK/Caddyfile.new" qspro-web-1:/tmp/Caddyfile.new
docker exec qspro-web-1 caddy validate --config /tmp/Caddyfile.new --adapter caddyfile
cat "$BK/Caddyfile.new" > "$CADDY"   # ghi đè giữ nguyên inode (Caddyfile gắn vào container dạng 1 file)
trap - ERR
if ! docker exec qspro-web-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile; then
  cat "$BK/Caddyfile.before" > "$CADDY"
  docker exec qspro-web-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile || true
  docker start nghiemthu-web
  echo "!! Caddy không nhận cấu hình mới: đã trả lại bản cũ"
  exit 1Duy
fi

cat > "$BK/rollback.txt" <<EOF
Quay lại bản Python cũ:
  cat $BK/Caddyfile.before > $CADDY
  docker exec qspro-web-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
  docker start nghiemthu-web
(Dữ liệu cũ nguyên vẹn trong volume nghiemthu_data, bản sao ở $BK/legacy.
 Lưu ý: những gì CTV/admin làm trên bản mới sau thời điểm $TS sẽ không có ở bản cũ.)
EOF

echo "== 5. kiểm tra qua tên miền"
sleep 2
curl -s -w " [%{http_code}]\n" https://nghiemthu.vuathau.com.vn/health
curl -s https://nghiemthu.vuathau.com.vn/api/v1/config | head -c 120; echo
curl -s -o /dev/null -w "web /login [%{http_code}]\n" https://nghiemthu.vuathau.com.vn/login
echo "Sao lưu + cách quay lại: $BK/rollback.txt"
