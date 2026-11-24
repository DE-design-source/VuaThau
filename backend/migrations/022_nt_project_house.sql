-- Nhà thầu chụp nhật ký vào công trình CTV đã chụp (bảng hiệu có SĐT nhà thầu): dự án của nhà thầu gắn với hồ sơ đó.
ALTER TABLE nt_projects ADD COLUMN IF NOT EXISTS house_id BIGINT REFERENCES houses(id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_nt_projects_user_house ON nt_projects (user_id, house_id) WHERE house_id IS NOT NULL AND deleted_at IS NULL;
