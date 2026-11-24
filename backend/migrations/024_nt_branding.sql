-- Logo trên ảnh (thiết kế "App – Logo trên ảnh" 24/11/2026): nhà thầu tải logo công ty hoặc nhập tên thương hiệu
-- (tối đa 20 ký tự) để in lên ảnh nhật ký; cỡ logo; giữ nhãn "Vua Thầu xác minh".
ALTER TABLE nt_profiles ADD COLUMN IF NOT EXISTS brand      TEXT    NOT NULL DEFAULT '';
ALTER TABLE nt_profiles ADD COLUMN IF NOT EXISTS logo_sha   TEXT    NOT NULL DEFAULT '';
ALTER TABLE nt_profiles ADD COLUMN IF NOT EXISTS logo_size  INT     NOT NULL DEFAULT 2;   -- 1 nhỏ, 2 vừa, 3 lớn
ALTER TABLE nt_profiles ADD COLUMN IF NOT EXISTS show_badge BOOLEAN NOT NULL DEFAULT TRUE;
