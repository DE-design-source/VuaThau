package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Customer: khách hàng của CTV kinh doanh (bảng customers).
type Customer struct {
	ID        int64     `json:"id"`
	CTVID     *int64    `json:"ctv_id"`
	CTVName   string    `json:"ctv_name"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // nha_pho | biet_thu | can_ho
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	Lat       *float64  `json:"lat"`
	Lng       *float64  `json:"lng"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CustomerRepository interface {
	// List: ctv > 0 chỉ khách của CTV đó.
	List(ctx context.Context, ctv int64) ([]Customer, error)
	Get(ctx context.Context, id int64) (*Customer, error)
	Create(ctx context.Context, c *Customer, by int64) error
	Update(ctx context.Context, c *Customer) error
	Delete(ctx context.Context, id int64) error
	// CTVKind: chức năng của CTV (users.kind), '' nếu không phải CTV.
	CTVKind(ctx context.Context, id int64) (string, error)
}

type customerRepository struct{ db *pgxpool.Pool }

func NewCustomerRepository(db *pgxpool.Pool) CustomerRepository { return &customerRepository{db: db} }

const customerCols = `c.id, c.ctv_id, COALESCE(u.name, ''), c.name, c.kind, c.phone, c.address, c.lat, c.lng, c.note, c.created_at, c.updated_at`

func scanCustomer(row pgx.Row) (*Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.CTVID, &c.CTVName, &c.Name, &c.Kind, &c.Phone, &c.Address, &c.Lat, &c.Lng, &c.Note, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *customerRepository) List(ctx context.Context, ctv int64) ([]Customer, error) {
	rows, err := r.db.Query(ctx, `SELECT `+customerCols+` FROM customers c LEFT JOIN users u ON u.id = c.ctv_id
		WHERE c.deleted_at IS NULL AND ($1 = 0 OR c.ctv_id = $1) ORDER BY c.updated_at DESC, c.id DESC`, ctv)
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
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *customerRepository) Get(ctx context.Context, id int64) (*Customer, error) {
	c, err := scanCustomer(r.db.QueryRow(ctx, `SELECT `+customerCols+` FROM customers c LEFT JOIN users u ON u.id = c.ctv_id
		WHERE c.id = $1 AND c.deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (r *customerRepository) Create(ctx context.Context, c *Customer, by int64) error {
	return r.db.QueryRow(ctx, `INSERT INTO customers (ctv_id, name, kind, phone, address, lat, lng, note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id, created_at, updated_at`,
		c.CTVID, c.Name, c.Kind, c.Phone, c.Address, c.Lat, c.Lng, c.Note, by).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *customerRepository) Update(ctx context.Context, c *Customer) error {
	tag, err := r.db.Exec(ctx, `UPDATE customers SET ctv_id = $2, name = $3, kind = $4, phone = $5, address = $6, lat = $7, lng = $8,
		note = $9, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		c.ID, c.CTVID, c.Name, c.Kind, c.Phone, c.Address, c.Lat, c.Lng, c.Note)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *customerRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `UPDATE customers SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *customerRepository) CTVKind(ctx context.Context, id int64) (string, error) {
	var k string
	err := r.db.QueryRow(ctx, `SELECT COALESCE(kind, '') FROM users WHERE id = $1 AND role = 'ctv' AND locked <> 2`, id).Scan(&k)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return k, err
}
