#!/usr/bin/env bash
# Chạy trên máy chủ: dựng và chạy vtnt-api. Không đụng dịch vụ khác.
set -euo pipefail
cd /opt/vuathau-nt

# Lấy các khoá tích hợp từ bản cũ (AI, Google, Drive) nếu .env mới chưa có. Không in giá trị.
OLD=/opt/vuathau-nghiemthu/.env
for k in ANTHROPIC_API_KEY ANTHROPIC_WORKSPACE_ID AI_MODEL GOOGLE_MAPS_API_KEY GOOGLE_CLIENT_IDS \
         GDRIVE_CLIENT_ID GDRIVE_CLIENT_SECRET GDRIVE_REFRESH_TOKEN GDRIVE_FOLDER; do
  if ! grep -q "^$k=" .env && grep -q "^$k=." "$OLD" 2>/dev/null; then
    grep "^$k=" "$OLD" | tail -1 >> .env
    echo "chép $k từ bản cũ"
  fi
done
# Bản cũ dùng thư mục Drive mặc định trong mã khi không đặt GDRIVE_FOLDER
if grep -q '^GDRIVE_REFRESH_TOKEN=.' .env && ! grep -q '^GDRIVE_FOLDER=' .env; then
  echo 'GDRIVE_FOLDER=1gCMugtc9LrQEXvTL1ebsfFoypdFKmpqQ' >> .env
  echo "đặt GDRIVE_FOLDER mặc định của bản cũ"
fi
chmod 600 .env

# Thư mục dữ liệu ghi được cho user app (uid 10001) trong container
src=$(docker volume inspect vtnt_data -f '{{.Mountpoint}}')
mkdir -p "$src/anh" "$src/bienlai"
chown -R 10001:10001 "$src"

docker compose build api
docker compose up -d api
for i in $(seq 1 30); do
  s=$(docker inspect -f '{{.State.Health.Status}}' vtnt-api 2>/dev/null || echo starting)
  [ "$s" = healthy ] && break
  sleep 3
done
echo "vtnt-api: $s"
docker logs --tail 8 vtnt-api
echo "--- kiểm tra"
curl -s -o /dev/null -w "health %{http_code}\n" http://127.0.0.1:8095/health
curl -s http://127.0.0.1:8095/api/v1/config | head -c 160; echo
curl -s -o /dev/null -w "web %{http_code}\n" http://127.0.0.1:8095/login
curl -s -X POST -H 'Content-Type: application/json' -d '{"login":"khongco","password":"x"}' http://127.0.0.1:8095/api/v1/auth/login; echo
docker exec qspro-web-1 wget -qO- http://vtnt-api:8080/api/v1/config >/dev/null && echo "Caddy (qspro-web-1) gọi được vtnt-api:8080"
docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' vtnt-api vtnt-db
