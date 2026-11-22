-- CTV từ form "Tuyển CTV tư vấn công trình" (Lark) là CTV kinh doanh (góp ý 22/11/2026): thêm chức năng
-- "CTV kinh doanh" vào danh sách, đổi các CTV đã tạo từ form. Chỉ chạy 1 lần (khoá sync_state), admin sửa sau đó giữ nguyên.
DO $$ BEGIN
IF NOT EXISTS (SELECT 1 FROM sync_state WHERE k = 'lark_ctv_sales_v1') THEN
    INSERT INTO ctv_kinds (name, pos)
    SELECT 'CTV kinh doanh', COALESCE((SELECT MAX(pos) + 1 FROM ctv_kinds), 1)
    WHERE NOT EXISTS (SELECT 1 FROM ctv_kinds WHERE name = 'CTV kinh doanh');
    UPDATE users SET kind = 'CTV kinh doanh'
    WHERE role = 'ctv' AND note LIKE 'Form tuyển CTV (Lark)%' AND COALESCE(kind, '') IN ('', 'CTV');
    INSERT INTO sync_state (k, v, at) VALUES ('lark_ctv_sales_v1', 'done', NOW());
END IF;
END $$;
