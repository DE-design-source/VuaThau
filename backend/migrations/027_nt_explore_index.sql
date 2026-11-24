-- Khám phá (GET /nt/explore): mỗi dòng nhật ký tìm nhà thầu trong bảng contractors theo SĐT chỉ còn chữ số.
-- Trước đây không có chỉ mục: 400 dòng = 400 lần quét cả bảng contractors. Câu truy vấn dùng đúng biểu thức này.
CREATE INDEX IF NOT EXISTS idx_contractors_phone_digits ON contractors ((regexp_replace(phone, '\D', '', 'g')));

-- Khám phá lọc nhật ký mới nhất của mọi dự án theo taken_at
CREATE INDEX IF NOT EXISTS idx_nt_logs_taken ON nt_logs (taken_at DESC);
