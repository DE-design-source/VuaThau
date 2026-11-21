-- Khách hàng (layout bản đồ 21/11/2026): lấy từ pipeline Kinh doanh của Dezon Pro (dezon.tech, /api/v1/sales/leads)
-- bằng khoá chỉ đọc (DEZON_TECH_KEY), mỗi 15 phút. Chỉ đọc bên Vua Thầu; vị trí: máy chủ tìm theo địa chỉ.
-- Khách bị xoá bên dezon.tech: đánh dấu deleted_at.
CREATE TABLE IF NOT EXISTS customers (
    id          BIGSERIAL PRIMARY KEY,
    ext_id      TEXT NOT NULL UNIQUE,          -- id khách bên dezon.tech
    name        TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL DEFAULT '',      -- loại công trình (Nhà phố, Biệt thự, Căn hộ...)
    scale       TEXT NOT NULL DEFAULT '',
    phone       TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    area        TEXT NOT NULL DEFAULT '',      -- khu vực / tỉnh bên dezon.tech
    stage       TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT '',
    owner       TEXT NOT NULL DEFAULT '',      -- người phụ trách (nhân viên kinh doanh Dezon)
    value_vnd   BIGINT NOT NULL DEFAULT 0,
    note        TEXT NOT NULL DEFAULT '',
    lat         DOUBLE PRECISION,
    lng         DOUBLE PRECISION,
    geo_q       TEXT NOT NULL DEFAULT '',      -- chuỗi đã dùng để tìm toạ độ (đổi địa chỉ thì tìm lại)
    lead_at     TIMESTAMPTZ,                   -- ngày tạo bên dezon.tech
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_customers_live ON customers (deleted_at) WHERE deleted_at IS NULL;
