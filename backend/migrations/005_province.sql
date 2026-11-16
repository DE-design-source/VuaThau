-- 005_province.sql
-- Tỉnh / thành của công trình theo 34 đơn vị hành chính cấp tỉnh từ 1/7/2025 (Nghị quyết 202/2025/QH15).
-- Mã theo OCHA cod-ab-vnm (VN01 Hà Nội, VN79 TP. Hồ Chí Minh...). API tự gán theo toạ độ / địa chỉ (cả tên tỉnh cũ).
-- Idempotent: safe to run more than once.
ALTER TABLE houses ADD COLUMN IF NOT EXISTS province VARCHAR(8);
CREATE INDEX IF NOT EXISTS idx_houses_province ON houses (province);
