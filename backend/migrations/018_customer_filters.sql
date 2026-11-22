-- Bộ lọc tab Khách hàng (góp ý 22/11/2026): tỉnh thành, trạng thái giao tiếp (1-5 sao bên Dezon Pro; 0 = chưa đánh giá).
ALTER TABLE customers ADD COLUMN IF NOT EXISTS province TEXT NOT NULL DEFAULT '';
ALTER TABLE customers ADD COLUMN IF NOT EXISTS rating   INT  NOT NULL DEFAULT 0;
