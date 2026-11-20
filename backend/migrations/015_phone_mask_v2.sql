-- Che SĐT (20/11/2026, bản 2): nhận cả SĐT OCR đọc dính biểu tượng điện thoại / nhãn ("©0935...", "Hotline:0905...").
-- Chạy 1 lần (đánh dấu bằng dòng sync_state 'phone_mask_v2'): quét lại các ảnh đã quét bằng bản 1. Ảnh đã che cũ
-- (<sha>_m.jpg) được tạo lại khi quét xong.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sync_state WHERE k = 'phone_mask_v2') THEN
        UPDATE photos SET phone_mask = NULL WHERE phone_mask IS NOT NULL;
        INSERT INTO sync_state (k, v, at) VALUES ('phone_mask_v2', 'done', NOW());
    END IF;
END $$;
