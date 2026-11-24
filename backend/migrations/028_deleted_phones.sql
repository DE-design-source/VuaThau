-- Người dùng tự xoá tài khoản (CTV, nhà thầu): giữ dấu SĐT đã xoá (sha256, không giữ số) để đồng bộ form Lark
-- (CTV duyệt tham gia, nhà thầu xác minh) không tự tạo lại tài khoản sau 15 phút.
-- Người đó tự đăng ký lại trong app thì vẫn được (chỉ việc tạo tự động bỏ qua).
CREATE TABLE IF NOT EXISTS deleted_phones (
    phone_hash TEXT PRIMARY KEY,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
