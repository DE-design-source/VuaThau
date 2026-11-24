-- Chế độ "Ẩn thông tin CTV, nhà thầu" trên web (góp ý 25/11/2026): che cả chữ trên ảnh (tên công ty trên bảng hiệu,
-- băng rôn, dải chữ in trên ảnh). Vùng chữ do OCR tìm cùng lượt quét SĐT; NULL = chưa quét (ảnh cũ được quét nền).
ALTER TABLE photos ADD COLUMN IF NOT EXISTS text_mask JSONB;
