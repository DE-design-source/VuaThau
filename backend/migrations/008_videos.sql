-- Video công trình (18/11/2026): CTV quay / chọn trên app, video Messenger kéo về cùng hồ sơ.
-- Chỉ để admin xem thêm: không qua AI, không tính vào số ảnh đạt / tiền. File: <DATA_DIR>/video/<sha256>.
CREATE TABLE IF NOT EXISTS videos (
    id         BIGSERIAL   PRIMARY KEY,
    house_id   BIGINT      NOT NULL REFERENCES houses (id) ON DELETE CASCADE,
    sha256     CHAR(64)    NOT NULL,
    mime       TEXT        NOT NULL DEFAULT 'video/mp4',
    size       BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT videos_sha256_key UNIQUE (sha256)
);
CREATE INDEX IF NOT EXISTS idx_videos_house ON videos (house_id);
