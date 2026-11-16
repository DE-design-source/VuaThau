#!/usr/bin/env bash
# Tài khoản chỉ đọc vtnt_reader trên DB dezon-inbox cho hệ thống nghiệm thu. Không in mật khẩu.
set -euo pipefail
ENV=/opt/vuathau-nt/.env
DB=dezon-inbox-db
PW=$(openssl rand -hex 24)
psql_admin() { docker exec -i "$DB" psql -U dezon -d dezon_inbox -v ON_ERROR_STOP=1 -q "$@"; }

psql_admin <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'vtnt_reader') THEN
    CREATE ROLE vtnt_reader LOGIN;
  END IF;
END \$\$;
ALTER ROLE vtnt_reader WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT 5 PASSWORD '$PW';
ALTER ROLE vtnt_reader SET default_transaction_read_only = on;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM vtnt_reader;
GRANT CONNECT ON DATABASE dezon_inbox TO vtnt_reader;
GRANT USAGE ON SCHEMA public TO vtnt_reader;
GRANT SELECT ON project_logs, conversations, contacts, messages TO vtnt_reader;
SQL
echo "đã tạo / cập nhật vtnt_reader"

# Kiểm: đọc được 4 bảng, không đọc được bảng khác, không ghi được
R() { docker exec -i -e PGPASSWORD="$PW" "$DB" psql -h 127.0.0.1 -U vtnt_reader -d dezon_inbox -At -c "$1" 2>&1; }
echo "đọc project_logs: $(R 'SELECT count(*) FROM project_logs')"
echo "đọc users (phải bị chặn): $(R 'SELECT count(*) FROM users' | head -1)"
echo "ghi (phải bị chặn): $(R "UPDATE contacts SET notes = notes WHERE false" | head -1)"
echo "tạo bảng (phải bị chặn): $(R 'CREATE TABLE vtnt_x (id int)' | head -1)"

# Đổi .env sang tài khoản mới (giữ host / db), sao lưu trước
cp -a "$ENV" "$ENV.bak-$(date +%Y%m%d-%H%M%S)"
sed -i -E "s#^INBOX_DATABASE_URL=postgres://[^@]+@#INBOX_DATABASE_URL=postgres://vtnt_reader:$PW@#" "$ENV"
chmod 600 "$ENV"
grep -q '^INBOX_DATABASE_URL=postgres://vtnt_reader:' "$ENV" && echo ".env đã dùng vtnt_reader"
cd /opt/vuathau-nt && docker compose up -d --force-recreate api 2>&1 | tail -1
