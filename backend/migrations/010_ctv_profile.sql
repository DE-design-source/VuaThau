-- Trang Cộng tác viên (19/11/2026): thêm Khu vực, Chức năng, Ghi chú cho CTV (admin sửa ở trang 1 CTV).
-- region: mã tỉnh admin chọn; '' = tự tính theo tỉnh CTV chụp nhiều công trình nhất.
-- kind: chức năng / loại CTV, chọn trong danh sách ctv_kinds (admin sửa ở Cấu hình).
-- note: ghi chú nội bộ của admin, không gửi cho app CTV.
ALTER TABLE users ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS kind   TEXT NOT NULL DEFAULT 'CTV';
ALTER TABLE users ADD COLUMN IF NOT EXISTS note   TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS ctv_kinds (
    name TEXT PRIMARY KEY,
    pos  INT  NOT NULL DEFAULT 0
);
-- lần đầu: 1 loại mặc định (chạy lại migration không thêm lại nếu admin đã sửa danh sách)
INSERT INTO ctv_kinds (name, pos) SELECT 'CTV', 0 WHERE NOT EXISTS (SELECT 1 FROM ctv_kinds);
