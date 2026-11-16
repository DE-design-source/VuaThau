-- 004_inbox_dup_map.sql
-- Hồ sơ CTV gửi qua Messenger (lấy tự động từ dezon-inbox), chặn chụp trùng công trình, bản đồ.
-- Idempotent: safe to run more than once.

-- Nguồn hồ sơ: app = CTV nộp trong app, inbox = chatbot Messenger ghi nhận (dezon-inbox project_logs)
ALTER TABLE houses ADD COLUMN IF NOT EXISTS source       VARCHAR(10) NOT NULL DEFAULT 'app';
ALTER TABLE houses ADD COLUMN IF NOT EXISTS ext_id       TEXT;          -- id project_logs bên dezon-inbox
ALTER TABLE houses ADD COLUMN IF NOT EXISTS dup_of       BIGINT REFERENCES houses (id) ON DELETE SET NULL;
ALTER TABLE houses ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ;   -- lần gửi duyệt đầu (tính hạn chụp trùng)
CREATE UNIQUE INDEX IF NOT EXISTS houses_ext_id_key ON houses (ext_id) WHERE ext_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_houses_submitted ON houses (submitted_at);
UPDATE houses SET submitted_at = created_at WHERE submitted_at IS NULL AND status <> 'nhap';

-- CTV tạo từ Messenger: id liên hệ (contacts.id) bên dezon-inbox
ALTER TABLE users ADD COLUMN IF NOT EXISTS inbox_contact TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS users_inbox_contact_key ON users (inbox_contact) WHERE inbox_contact IS NOT NULL;

-- Chặn chụp trùng: cùng công trình (trong bán kính, AI so ảnh) đã có hồ sơ trong số ngày này
INSERT INTO settings (k, v) VALUES ('dup_days', 30), ('dup_radius_m', 50) ON CONFLICT (k) DO NOTHING;

-- Trạng thái đồng bộ (con trỏ project_logs đã lấy, lần chạy gần nhất, lỗi)
CREATE TABLE IF NOT EXISTS sync_state (
    k  TEXT        PRIMARY KEY,
    v  TEXT        NOT NULL DEFAULT '',
    at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
