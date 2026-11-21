-- Khách hàng của CTV kinh doanh (layout bản đồ 21/11/2026): CTV kinh doanh thêm trong app, admin quản lý trên web.
-- Loại: nha_pho | biet_thu | can_ho. Vị trí: CTV ghim tại chỗ (GPS) hoặc máy chủ tìm theo địa chỉ. Xoá mềm (deleted_at).
CREATE TABLE IF NOT EXISTS customers (
    id          BIGSERIAL PRIMARY KEY,
    ctv_id      BIGINT REFERENCES users(id),
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'nha_pho',
    phone       TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    lat         DOUBLE PRECISION,
    lng         DOUBLE PRECISION,
    note        TEXT NOT NULL DEFAULT '',
    created_by  BIGINT REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_customers_ctv ON customers (ctv_id) WHERE deleted_at IS NULL;
