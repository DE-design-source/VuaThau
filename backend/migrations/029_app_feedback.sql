-- Đánh giá app (góp ý 25/11/2026): CTV / nhà thầu chụp đủ 30 ảnh thì app hỏi 1-5 sao + góp ý.
-- later = true: bấm "Để sau" (hỏi lại 1 lần khi đủ 60 ảnh); stars NULL khi later.
CREATE TABLE IF NOT EXISTS app_feedback (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stars      SMALLINT CHECK (stars BETWEEN 1 AND 5),
    comment    TEXT NOT NULL DEFAULT '',
    later      BOOLEAN NOT NULL DEFAULT false,
    photos     INT NOT NULL DEFAULT 0,      -- số ảnh người đó đã tải lên lúc trả lời
    version    TEXT NOT NULL DEFAULT '',    -- phiên bản app
    platform   TEXT NOT NULL DEFAULT '',    -- android / web
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_app_feedback_user ON app_feedback (user_id);
CREATE INDEX IF NOT EXISTS idx_app_feedback_created ON app_feedback (created_at DESC);
