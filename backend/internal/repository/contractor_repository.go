package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RegContractor: nhà thầu đăng ký qua form Vua Thầu (bảng contractors).
type RegContractor struct {
	ID         int64     `json:"id"`
	LarkRecord string    `json:"-"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Role       string    `json:"role"`
	Contact    string    `json:"contact"`
	Phone      string    `json:"phone"`
	Email      string    `json:"email"`
	Areas      string    `json:"areas"`
	Exp        string    `json:"exp"`
	Staff      string    `json:"staff"`
	ValueRange string    `json:"value_range"`
	Fields     []string  `json:"fields"`
	Verified   bool      `json:"verified"`
	Note       string    `json:"note"`
	JoinedAt   time.Time `json:"joined_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ContractorRepository interface {
	List(ctx context.Context) ([]RegContractor, error)
	// UpsertLark: thêm / cập nhật theo mã dòng Lark.
	UpsertLark(ctx context.Context, c *RegContractor) error
}

type contractorRepository struct{ db *pgxpool.Pool }

func NewContractorRepository(db *pgxpool.Pool) ContractorRepository {
	return &contractorRepository{db: db}
}

func (r *contractorRepository) List(ctx context.Context) ([]RegContractor, error) {
	rows, err := r.db.Query(ctx, `SELECT id, COALESCE(lark_record, ''), name, kind, role, contact, phone, email, areas, exp, staff,
		value_range, fields, verified, note, joined_at, updated_at FROM contractors ORDER BY joined_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RegContractor{}
	for rows.Next() {
		var c RegContractor
		if err := rows.Scan(&c.ID, &c.LarkRecord, &c.Name, &c.Kind, &c.Role, &c.Contact, &c.Phone, &c.Email, &c.Areas, &c.Exp,
			&c.Staff, &c.ValueRange, &c.Fields, &c.Verified, &c.Note, &c.JoinedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *contractorRepository) UpsertLark(ctx context.Context, c *RegContractor) error {
	if c.Fields == nil {
		c.Fields = []string{}
	}
	_, err := r.db.Exec(ctx, `INSERT INTO contractors (lark_record, name, kind, role, contact, phone, email, areas, exp, staff,
			value_range, fields, verified, note, joined_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (lark_record) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind, role = EXCLUDED.role,
			contact = EXCLUDED.contact, phone = EXCLUDED.phone, email = EXCLUDED.email, areas = EXCLUDED.areas, exp = EXCLUDED.exp,
			staff = EXCLUDED.staff, value_range = EXCLUDED.value_range, fields = EXCLUDED.fields, verified = EXCLUDED.verified,
			note = EXCLUDED.note, joined_at = EXCLUDED.joined_at,
			updated_at = CASE WHEN (contractors.name, contractors.phone, contractors.areas, contractors.verified, contractors.fields)
				IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.phone, EXCLUDED.areas, EXCLUDED.verified, EXCLUDED.fields)
				THEN now() ELSE contractors.updated_at END`,
		c.LarkRecord, c.Name, c.Kind, c.Role, c.Contact, c.Phone, c.Email, c.Areas, c.Exp, c.Staff, c.ValueRange, c.Fields,
		c.Verified, c.Note, c.JoinedAt)
	return err
}
