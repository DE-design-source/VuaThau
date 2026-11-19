-- Thông tin CTV bên Messenger (dezon-inbox, khung "Thông tin khách"): tên, SĐT, email, nhãn, ghi chú, các ô chatbot
-- ghi nhận (nhu cầu, khu vực, số tài khoản...). Đồng bộ mỗi lượt đọc Messenger, hiện ở trang 1 CTV; ô của web còn
-- trống thì điền từ đây (khu vực, số tài khoản, ghi chú), ô admin đã nhập không bị ghi đè.
ALTER TABLE users ADD COLUMN IF NOT EXISTS inbox_info JSONB;
ALTER TABLE users ADD COLUMN IF NOT EXISTS inbox_at   TIMESTAMPTZ;
