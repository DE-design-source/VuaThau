-- Che SĐT trên ảnh công trình (bảng hiệu nhà thầu...) khi sale / CTV / Google Drive xem; admin, kế toán xem ảnh gốc.
-- phone_mask: các vùng có SĐT (toạ độ tỉ lệ 0..1) do OCR Tesseract tìm; NULL = chưa quét, [] = không có SĐT.
ALTER TABLE photos ADD COLUMN IF NOT EXISTS phone_mask JSONB;
CREATE INDEX IF NOT EXISTS idx_photos_no_mask ON photos (id) WHERE phone_mask IS NULL;
