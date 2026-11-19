-- Loại chụp của hồ sơ (19/11/2026): chỉ chụp bên ngoài công trình được tính tiền thấp hơn chụp cả trong lẫn ngoài.
-- scope: admin chọn khi duyệt ('' = theo AI gợi ý; hồ sơ trước ngày này để trống = tính tiền như cũ).
-- scope_ai: AI gợi ý từ ảnh nộp qua app ('trong_ngoai' khi thấy ít nhất 1 ảnh chụp bên trong, không thì 'ngoai').
-- Giá tối đa của hồ sơ chỉ chụp ngoài nằm trong bảng settings (k = 'price_out'), mặc định trong mã: 35.000đ.
ALTER TABLE houses ADD COLUMN IF NOT EXISTS scope    TEXT NOT NULL DEFAULT '';
ALTER TABLE houses ADD COLUMN IF NOT EXISTS scope_ai TEXT NOT NULL DEFAULT '';
