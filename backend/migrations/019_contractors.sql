-- Nhà thầu đăng ký (góp ý 23/11/2026): đồng bộ từ Lark Base "Form Đăng ký nhà thầu - Vua Thầu".
-- Trang Nhà thầu của admin gộp bảng này với nhà thầu trích từ ảnh CTV chụp (houses.contractor_*), khớp theo SĐT.
CREATE TABLE IF NOT EXISTS contractors (
    id          BIGSERIAL PRIMARY KEY,
    lark_record TEXT UNIQUE,                 -- mã dòng trên Lark (NULL: admin thêm tay)
    name        TEXT NOT NULL DEFAULT '',    -- Tên công ty / cá nhân
    kind        TEXT NOT NULL DEFAULT '',    -- Loại hình: Công ty, Tổ đội, Cá nhân
    role        TEXT NOT NULL DEFAULT '',    -- Vai trò: Tổng thầu, Thầu phụ...
    contact     TEXT NOT NULL DEFAULT '',    -- người liên hệ
    phone       TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    areas       TEXT NOT NULL DEFAULT '',    -- tỉnh / thành nhận thi công
    exp         TEXT NOT NULL DEFAULT '',    -- số năm kinh nghiệm
    staff       TEXT NOT NULL DEFAULT '',    -- quy mô nhân sự
    value_range TEXT NOT NULL DEFAULT '',    -- giá trị công trình thường nhận
    fields      TEXT[] NOT NULL DEFAULT '{}',-- lĩnh vực thi công
    verified    BOOLEAN NOT NULL DEFAULT false,
    note        TEXT NOT NULL DEFAULT '',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
