package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Customer: khách hàng lấy từ pipeline Kinh doanh của Dezon Pro (bảng customers, chỉ đọc bên Vua Thầu).
type Customer struct {
	ID       int64      `json:"id"`
	ExtID    string     `json:"ext_id"`
	Name     string     `json:"name"`
	Kind     string     `json:"kind"` // loại công trình: Nhà phố, Biệt thự, Căn hộ...
	Scale    string     `json:"scale"`
	Phone    string     `json:"phone"`
	Email    string     `json:"email"`
	Address  string     `json:"address"`
	Area     string     `json:"area"`
	Stage    string     `json:"stage"`
	Source   string     `json:"source"`
	Owner    string     `json:"owner"` // người phụ trách bên Dezon
	ValueVnd int64      `json:"value_vnd"`
	Note     string     `json:"note"`
	Lat      *float64   `json:"lat"`
	Lng      *float64   `json:"lng"`
	LeadAt   *time.Time `json:"lead_at"`
	SyncedAt time.Time  `json:"synced_at"`
	GeoQ     string     `json:"-"`
}

type CustomerRepository interface {
	List(ctx context.Context) ([]Customer, error)
	// Upsert: thêm / cập nhật theo ExtID (giữ toạ độ cũ nếu chuỗi tìm toạ độ không đổi).
	Upsert(ctx context.Context, c *Customer) error
	// Prune: đánh dấu xoá khách không còn bên Dezon Pro (keep: ExtID còn).
	Prune(ctx context.Context, keep []string) (int64, error)
	// NeedGeo: khách có địa chỉ nhưng chưa tìm toạ độ theo chuỗi hiện tại.
	NeedGeo(ctx context.Context, limit int) ([]Customer, error)
	SetGeo(ctx context.Context, id int64, q string, lat, lng *float64) error
}

type customerRepository struct{ db *pgxpool.Pool }

func NewCustomerRepository(db *pgxpool.Pool) CustomerRepository { return &customerRepository{db: db} }

const customerCols = `id, ext_id, name, kind, scale, phone, email, address, area, stage, source, owner, value_vnd, note, lat, lng, lead_at, synced_at, geo_q`

type rowScanner interface{ Scan(dest ...any) error }

func scanCustomer(row rowScanner) (Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.ExtID, &c.Name, &c.Kind, &c.Scale, &c.Phone, &c.Email, &c.Address, &c.Area, &c.Stage, &c.Source, &c.Owner,
		&c.ValueVnd, &c.Note, &c.Lat, &c.Lng, &c.LeadAt, &c.SyncedAt, &c.GeoQ)
	return c, err
}

func (r *customerRepository) query(ctx context.Context, sql string, args ...any) ([]Customer, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *customerRepository) List(ctx context.Context) ([]Customer, error) {
	return r.query(ctx, `SELECT `+customerCols+` FROM customers WHERE deleted_at IS NULL ORDER BY lead_at DESC NULLS LAST, id DESC`)
}

func (r *customerRepository) Upsert(ctx context.Context, c *Customer) error {
	_, err := r.db.Exec(ctx, `INSERT INTO customers (ext_id, name, kind, scale, phone, email, address, area, stage, source, owner, value_vnd, note, lead_at, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
		ON CONFLICT (ext_id) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind, scale = EXCLUDED.scale, phone = EXCLUDED.phone,
			email = EXCLUDED.email, address = EXCLUDED.address, area = EXCLUDED.area, stage = EXCLUDED.stage, source = EXCLUDED.source,
			owner = EXCLUDED.owner, value_vnd = EXCLUDED.value_vnd, note = EXCLUDED.note, lead_at = EXCLUDED.lead_at, synced_at = NOW(),
			deleted_at = NULL`,
		c.ExtID, c.Name, c.Kind, c.Scale, c.Phone, c.Email, c.Address, c.Area, c.Stage, c.Source, c.Owner, c.ValueVnd, c.Note, c.LeadAt)
	return err
}

func (r *customerRepository) Prune(ctx context.Context, keep []string) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE customers SET deleted_at = NOW() WHERE deleted_at IS NULL AND NOT (ext_id = ANY($1))`, keep)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *customerRepository) NeedGeo(ctx context.Context, limit int) ([]Customer, error) {
	return r.query(ctx, `SELECT `+customerCols+` FROM customers
		WHERE deleted_at IS NULL AND (address <> '' OR area <> '') AND geo_q <> concat_ws(', ', NULLIF(address, ''), NULLIF(area, ''))
		ORDER BY id LIMIT $1`, limit)
}

func (r *customerRepository) SetGeo(ctx context.Context, id int64, q string, lat, lng *float64) error {
	_, err := r.db.Exec(ctx, `UPDATE customers SET geo_q = $2, lat = $3, lng = $4 WHERE id = $1`, id, q, lat, lng)
	return err
}
