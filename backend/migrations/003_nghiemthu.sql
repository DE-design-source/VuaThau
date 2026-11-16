-- 003_nghiemthu.sql
-- Nghiệp vụ nghiệm thu: CTV nộp hồ sơ ảnh -> AI kiểm -> admin duyệt -> kế toán thanh toán.
-- Mở rộng bảng users (thêm vai trò, ngân hàng, Google, khoá) và tạo các bảng hồ sơ, ảnh, phiếu chi...
-- Idempotent: safe to run more than once.

-- ---------- users: dùng chung cho CTV, admin, kế toán ----------
-- username = tên đăng nhập: SĐT với CTV, SĐT hoặc tên (a-z0-9_) với nhân viên.
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS name          VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS role          VARCHAR(10)  NOT NULL DEFAULT 'ctv';
ALTER TABLE users ADD COLUMN IF NOT EXISTS bank          TEXT         NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS google_sub    TEXT;
-- 0 = bình thường, 1 = admin khoá, 2 = CTV tự xoá tài khoản
ALTER TABLE users ADD COLUMN IF NOT EXISTS locked        SMALLINT     NOT NULL DEFAULT 0;
-- tăng khi đổi mật khẩu / khoá: token cũ hết hiệu lực
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INTEGER      NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_role_check') THEN
        ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('ctv', 'admin', 'ketoan'));
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS users_google_sub_key ON users (google_sub) WHERE google_sub IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_role ON users (role);

-- ---------- phiếu chi: mỗi lần kế toán trả cho 1 CTV ----------
CREATE TABLE IF NOT EXISTS batches (
    id         BIGSERIAL   PRIMARY KEY,
    -- da_chi = đã thanh toán, huy = phiếu ghi nhầm đã huỷ (giữ để đối soát)
    status     VARCHAR(10) NOT NULL DEFAULT 'da_chi' CHECK (status IN ('da_chi', 'huy')),
    total      BIGINT      NOT NULL,
    ref        TEXT        NOT NULL DEFAULT '',   -- mã giao dịch ngân hàng
    receipt    TEXT        NOT NULL DEFAULT '',   -- tên file ảnh biên lai
    ctv_id     BIGINT      REFERENCES users (id),
    created_by BIGINT      REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_batches_ctv ON batches (ctv_id);
CREATE INDEX IF NOT EXISTS idx_batches_created ON batches (created_at);

-- ---------- hồ sơ: 1 căn nhà đang xây ----------
CREATE TABLE IF NOT EXISTS houses (
    id               BIGSERIAL    PRIMARY KEY,
    ctv_id           BIGINT       NOT NULL REFERENCES users (id),
    status           VARCHAR(10)  NOT NULL DEFAULT 'nhap'
                     CHECK (status IN ('nhap', 'tra_lai', 'cho_duyet', 'tu_choi', 'da_duyet', 'da_chi')),
    address          TEXT         NOT NULL DEFAULT '',
    lat              DOUBLE PRECISION,
    lng              DOUBLE PRECISION,
    contractor_name  TEXT         NOT NULL DEFAULT '',
    contractor_phone TEXT         NOT NULL DEFAULT '',
    floors           INTEGER,
    stage            TEXT         NOT NULL DEFAULT '',
    note             TEXT         NOT NULL DEFAULT '',   -- lý do trả lại / từ chối
    dup              TEXT         NOT NULL DEFAULT '',   -- cảnh báo nhà gần hồ sơ khác
    amount           BIGINT       NOT NULL DEFAULT 0,
    -- '' chưa đọc, dang_doc, xong, loi
    ai_state         VARCHAR(10)  NOT NULL DEFAULT '',
    ai_json          JSONB,                              -- giá trị AI đã điền (để gắn nhãn "AI đọc")
    batch_id         BIGINT       REFERENCES batches (id),
    -- Google Drive: '' chưa đẩy, 'cho' chờ đẩy, còn lại là id thư mục
    drive            TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_houses_ctv_status ON houses (ctv_id, status);
CREATE INDEX IF NOT EXISTS idx_houses_status ON houses (status);
CREATE INDEX IF NOT EXISTS idx_houses_batch ON houses (batch_id);
CREATE INDEX IF NOT EXISTS idx_houses_drive ON houses (drive) WHERE drive = 'cho';

DROP TRIGGER IF EXISTS trg_houses_set_updated_at ON houses;
CREATE TRIGGER trg_houses_set_updated_at
    BEFORE UPDATE ON houses
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- ---------- ảnh của hồ sơ ----------
CREATE TABLE IF NOT EXISTS photos (
    id        BIGSERIAL PRIMARY KEY,
    house_id  BIGINT    NOT NULL REFERENCES houses (id) ON DELETE CASCADE,
    sha256    CHAR(64)  NOT NULL,
    dhash     CHAR(16)  NOT NULL,              -- băm cảm nhận, chống nộp lại ảnh đã resize/nén
    exif_time TEXT      NOT NULL DEFAULT '',   -- "YYYY-MM-DD HH:MM" giờ Việt Nam
    ai_time   TEXT      NOT NULL DEFAULT '',   -- giờ AI đọc trên dải chữ Timemark
    lat       DOUBLE PRECISION,
    lng       DOUBLE PRECISION,
    address   TEXT      NOT NULL DEFAULT '',
    is_site   BOOLEAN,                         -- AI: có phải công trình đang xây
    dup_of    BIGINT,                          -- id hồ sơ có ảnh gần giống
    mime      TEXT      NOT NULL DEFAULT 'image/jpeg',
    CONSTRAINT photos_sha256_key UNIQUE (sha256)
);
CREATE INDEX IF NOT EXISTS idx_photos_house ON photos (house_id);

-- ---------- lịch sử thao tác ----------
CREATE TABLE IF NOT EXISTS house_logs (
    id       BIGSERIAL   PRIMARY KEY,
    house_id BIGINT      REFERENCES houses (id) ON DELETE CASCADE,
    batch_id BIGINT      REFERENCES batches (id),
    user_id  BIGINT      REFERENCES users (id),
    action   TEXT        NOT NULL,
    note     TEXT        NOT NULL DEFAULT '',
    at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_house_logs_house ON house_logs (house_id);
CREATE INDEX IF NOT EXISTS idx_house_logs_batch ON house_logs (batch_id);
CREATE INDEX IF NOT EXISTS idx_house_logs_at ON house_logs (at);

-- ---------- thưởng (+) / trừ (-) tiền, cộng vào lần thanh toán tới ----------
CREATE TABLE IF NOT EXISTS adjustments (
    id         BIGSERIAL   PRIMARY KEY,
    ctv_id     BIGINT      NOT NULL REFERENCES users (id),
    amount     BIGINT      NOT NULL CHECK (amount <> 0),
    note       TEXT        NOT NULL,
    batch_id   BIGINT      REFERENCES batches (id),
    created_by BIGINT      REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_adjustments_ctv ON adjustments (ctv_id);
CREATE INDEX IF NOT EXISTS idx_adjustments_batch ON adjustments (batch_id);

-- ---------- cấu hình admin sửa được ----------
CREATE TABLE IF NOT EXISTS settings (
    k TEXT   PRIMARY KEY,
    v BIGINT NOT NULL
);
INSERT INTO settings (k, v) VALUES
    ('price_min', 35000), ('price_more', 50000), ('min_photos', 5), ('pay_min', 5)
ON CONFLICT (k) DO NOTHING;

-- ---------- kết quả AI đọc từng ảnh, theo sha256 (đọc 1 lần, dùng lại lúc nộp) ----------
CREATE TABLE IF NOT EXISTS ai_cache (
    sha256 CHAR(64)    PRIMARY KEY,
    data   JSONB       NOT NULL,
    at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
