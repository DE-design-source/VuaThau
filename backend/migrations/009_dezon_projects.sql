-- Công trình nhà thầu đăng trên dezon.vn/nha-thau/du-an-moi (19/11/2026): hiện thành lớp ghim riêng trên bản đồ
-- admin + sale. Không phải hồ sơ CTV: không tính tiền, không chặn trùng, không vào số liệu. Toạ độ tra từ địa chỉ
-- (geo_src: google | osm; geo_level: dia_chi = theo số nhà / đường, khu_vuc = theo phường / xã).
CREATE TABLE IF NOT EXISTS dezon_projects (
    id             BIGSERIAL        PRIMARY KEY,
    url            TEXT             NOT NULL,
    title          TEXT             NOT NULL DEFAULT '',
    contractor     TEXT             NOT NULL DEFAULT '',
    contractor_url TEXT             NOT NULL DEFAULT '',
    cover          TEXT             NOT NULL DEFAULT '',
    posted_at      DATE,
    lat            DOUBLE PRECISION,
    lng            DOUBLE PRECISION,
    province       TEXT             NOT NULL DEFAULT '',
    geo_src        TEXT             NOT NULL DEFAULT '',
    geo_level      TEXT             NOT NULL DEFAULT '',
    geo_query      TEXT             NOT NULL DEFAULT '',
    geo_at         TIMESTAMPTZ,
    first_seen     TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    last_seen      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    CONSTRAINT dezon_projects_url_key UNIQUE (url)
);
CREATE INDEX IF NOT EXISTS idx_dezon_projects_province ON dezon_projects (province);
