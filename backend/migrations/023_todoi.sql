-- Đăng ký trong app có 3 lựa chọn CTV / Nhà thầu / Tổ đội (góp ý slide 24/11/2026): tổ đội đăng ký như CTV, loại "Tổ đội".
INSERT INTO ctv_kinds (name, pos) SELECT 'Tổ đội', COALESCE((SELECT MAX(pos) FROM ctv_kinds), 0) + 1
WHERE NOT EXISTS (SELECT 1 FROM ctv_kinds WHERE name = 'Tổ đội');
