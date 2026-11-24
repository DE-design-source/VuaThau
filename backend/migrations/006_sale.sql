-- Vai trò sale: chỉ xem bản đồ, dự án, nhà thầu (không thấy CTV, không thấy tiền).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
-- deploy chạy lại mọi migration theo thứ tự: danh sách phải gồm cả vai trò thêm sau (nhathau, 020), không thì lỗi khi đã có tài khoản nhà thầu
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('ctv', 'admin', 'ketoan', 'sale', 'nhathau'));
