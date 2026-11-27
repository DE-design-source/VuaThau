-- Ảnh đại diện công trình (góp ý Lark 26/11/2026): AI chấm mỗi ảnh 0..10 độ hợp làm ảnh đại diện
-- (toàn cảnh mặt tiền, rõ nét, đúng chiều, thấy rõ là công trình). 0 = chưa chấm (ảnh cũ) -> lấy ảnh đầu như trước.
ALTER TABLE photos ADD COLUMN IF NOT EXISTS cover_score SMALLINT NOT NULL DEFAULT 0;
