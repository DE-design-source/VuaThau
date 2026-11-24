-- Form "Đăng tải dự án mới" (thiết kế 24/11/2026): địa chỉ tách Tỉnh / Phường / Số nhà, và ảnh dự án (ảnh bìa ở ô đầu, tối đa 20).
ALTER TABLE nt_projects ADD COLUMN IF NOT EXISTS province TEXT NOT NULL DEFAULT '';
ALTER TABLE nt_projects ADD COLUMN IF NOT EXISTS ward     TEXT NOT NULL DEFAULT '';
ALTER TABLE nt_projects ADD COLUMN IF NOT EXISTS street   TEXT NOT NULL DEFAULT '';

-- Ảnh dự án không thuộc nhật ký nào: log_id NULL
ALTER TABLE nt_media ALTER COLUMN log_id DROP NOT NULL;
