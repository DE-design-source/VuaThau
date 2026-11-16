-- Vai trò sale: chỉ xem bản đồ, dự án, nhà thầu (không thấy CTV, không thấy tiền).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('ctv', 'admin', 'ketoan', 'sale'));
