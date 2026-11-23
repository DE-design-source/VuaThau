-- Tài khoản nhà thầu trên app (thiết kế "App Vua Thầu Map – Nhà thầu", 23/11/2026): nhà thầu đăng nhập bằng SĐT,
-- tự đăng dự án (hồ sơ năng lực) và chụp nhật ký thi công hằng ngày (ảnh / video có giờ + GPS + mô tả).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('ctv', 'admin', 'ketoan', 'sale', 'nhathau'));

-- Hồ sơ công ty của nhà thầu
CREATE TABLE IF NOT EXISTS nt_profiles (
    user_id     BIGINT PRIMARY KEY REFERENCES users(id),
    company     TEXT    NOT NULL DEFAULT '',
    specialties TEXT[]  NOT NULL DEFAULT '{}',   -- Nhà phố, Biệt thự, Sửa chữa...
    regions     TEXT    NOT NULL DEFAULT '',     -- khu vực hoạt động: "TP.HCM · Bình Dương"
    years       INT     NOT NULL DEFAULT 0,      -- số năm kinh nghiệm
    verified    BOOLEAN NOT NULL DEFAULT FALSE,  -- Vua Thầu đã xác minh
    rating      NUMERIC(2,1) NOT NULL DEFAULT 0,
    reviews     INT     NOT NULL DEFAULT 0,
    views       INT     NOT NULL DEFAULT 0,      -- lượt xem hồ sơ
    lark_record TEXT    NOT NULL DEFAULT '',     -- dòng form đăng ký nhà thầu trên Lark (nếu có)
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Dự án (công trình) của nhà thầu
CREATE TABLE IF NOT EXISTS nt_projects (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT  NOT NULL REFERENCES users(id),
    name        TEXT    NOT NULL,
    address     TEXT    NOT NULL DEFAULT '',
    kind        TEXT    NOT NULL DEFAULT '',     -- loại công trình
    status      TEXT    NOT NULL DEFAULT 'dang_thi_cong' CHECK (status IN ('dang_thi_cong', 'hoan_thanh')),
    progress    INT     NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    started_on  DATE    NOT NULL DEFAULT CURRENT_DATE,
    finished_on DATE,
    lat         DOUBLE PRECISION,
    lng         DOUBLE PRECISION,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_nt_projects_user ON nt_projects (user_id) WHERE deleted_at IS NULL;

-- Nhật ký thi công: 1 lần cập nhật (mô tả + ảnh / video chụp cùng lúc)
CREATE TABLE IF NOT EXISTS nt_logs (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES nt_projects(id),
    note        TEXT   NOT NULL DEFAULT '',
    taken_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- giờ chụp (in trên ảnh)
    lat         DOUBLE PRECISION,
    lng         DOUBLE PRECISION,
    address     TEXT   NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nt_logs_project ON nt_logs (project_id, taken_at DESC);

-- Ảnh / video của nhật ký (file: DATA_DIR/anh, DATA_DIR/video theo sha256, như hồ sơ CTV)
CREATE TABLE IF NOT EXISTS nt_media (
    id          BIGSERIAL PRIMARY KEY,
    log_id      BIGINT NOT NULL REFERENCES nt_logs(id),
    project_id  BIGINT NOT NULL REFERENCES nt_projects(id),
    kind        TEXT   NOT NULL CHECK (kind IN ('photo', 'video')),
    sha256      TEXT   NOT NULL,
    mime        TEXT   NOT NULL DEFAULT '',
    size        BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, sha256)
);
CREATE INDEX IF NOT EXISTS idx_nt_media_log ON nt_media (log_id);
