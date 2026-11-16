#!/usr/bin/env bash
# Chạy trên máy chủ: tạo CSDL + lưu trữ cho Vua Thầu nghiệm thu (backend Go). Không đụng dịch vụ khác.
set -euo pipefail
D=/opt/vuathau-nt
cd "$D"
command -v rsync >/dev/null || { echo "thiếu rsync"; exit 1; }

# Mật khẩu ngẫu nhiên, chỉ tạo 1 lần (chạy lại không đổi)
if [ ! -f .env ]; then
  umask 077
  PW=$(openssl rand -hex 24)
  JWT=$(openssl rand -base64 48 | tr -d '\n')
  cat > .env <<EOF
# Bí mật của Vua Thầu nghiệm thu. KHÔNG đưa lên git.
POSTGRES_PASSWORD=$PW
# Chuỗi kết nối cho backend Go (trong mạng docker vtnt)
DATABASE_URL=postgres://vuathau:$PW@vtnt-db:5432/vuathau?sslmode=disable
# Từ máy chủ (ngoài docker) / SSH tunnel: postgres://vuathau:...@127.0.0.1:5434/vuathau
JWT_SECRET=$JWT
DATA_DIR=/data
EOF
  chmod 600 .env
  echo "đã tạo .env"
fi

docker compose up -d
for i in $(seq 1 30); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' vtnt-db)" = healthy ] && break
  sleep 2
done
docker inspect -f 'vtnt-db: {{.State.Health.Status}}' vtnt-db

# Migration (idempotent): chạy lần lượt trong 1 giao dịch mỗi file
for f in migrations/*.sql; do
  docker exec -i vtnt-db psql -q -v ON_ERROR_STOP=1 --single-transaction -U vuathau -d vuathau < "$f"
  echo "==> $f"
done

# Thư mục ảnh trong volume dữ liệu (DATA_DIR): quyền cho user app (uid 100 trong image alpine) ghi được
src=$(docker volume inspect vtnt_data -f '{{.Mountpoint}}')
mkdir -p "$src/anh" "$src/bienlai"

# Sao lưu mỗi đêm 02:45
install -m 700 backup.sh /usr/local/bin/vtnt-backup
cat > /etc/cron.d/vtnt-backup <<'EOF'
# Vua Thầu nghiệm thu: sao lưu CSDL + ảnh mỗi đêm
45 2 * * * root /usr/local/bin/vtnt-backup >> /var/log/vtnt-backup.log 2>&1
EOF
chmod 644 /etc/cron.d/vtnt-backup
/usr/local/bin/vtnt-backup

echo "--- kiểm tra"
docker exec vtnt-db psql -U vuathau -d vuathau -Atc "SHOW server_encoding; SELECT string_agg(tablename, ', ' ORDER BY tablename) FROM pg_tables WHERE schemaname='public';"
docker exec vtnt-db psql -U vuathau -d vuathau -Atc "SELECT k || '=' || v FROM settings ORDER BY k"
docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' vtnt-db
