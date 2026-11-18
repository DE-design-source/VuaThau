package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

// CTVRow: 1 CTV trong danh sách quản lý, kèm số liệu hồ sơ và tiền.
type CTVRow struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Locked    int        `json:"locked"`
	CreatedAt time.Time  `json:"created_at"`
	Houses    int        `json:"houses"`
	Waiting   int        `json:"waiting"`
	Approved  int        `json:"approved"`
	Paid      int64      `json:"paid"`
	Owed      int64      `json:"owed"`
	LastAt    *time.Time `json:"last_at"`
}

type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	ByInboxContact(ctx context.Context, contact string) (*model.User, error)
	SetInboxContact(ctx context.Context, id int64, contact string) error

	UpdateBank(ctx context.Context, id int64, bank string) error
	// UpdatePassword đổi mật khẩu và tăng token_version (token cũ hết hiệu lực).
	UpdatePassword(ctx context.Context, id int64, hash string) error
	// SetLocked khoá / mở khoá và tăng token_version.
	SetLocked(ctx context.Context, id int64, locked int) error
	// Anonymize: CTV tự xoá tài khoản. Xoá tên, SĐT, email, Google, ngân hàng; hồ sơ, phiếu chi giữ lại.
	Anonymize(ctx context.Context, id int64, hash string) error

	ListStaff(ctx context.Context) ([]model.User, error)
	ListCTVs(ctx context.Context, q string, limit, offset int) ([]CTVRow, int, error)
	CountCTVs(ctx context.Context) (int, error)
	CountNewCTVs(ctx context.Context, from, to time.Time) (int, error)
}

type userRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return &userRepository{db: db}
}

const userCols = `id, username, name, role, COALESCE(email, ''), bank, locked, COALESCE(google_sub, ''), token_version,
	password_hash, created_at, updated_at`

func scanUser(row pgx.Row) (*model.User, error) {
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Email, &u.Bank, &u.Locked, &u.GoogleSub, &u.TokenVersion,
		&u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

// ByInboxContact: CTV đã ghép với liên hệ Messenger (contacts.id bên dezon-inbox).
func (r *userRepository) ByInboxContact(ctx context.Context, contact string) (*model.User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE inbox_contact = $1`, contact))
}

func (r *userRepository) SetInboxContact(ctx context.Context, id int64, contact string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET inbox_contact = $2 WHERE id = $1`, id, contact)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	return err
}

func (r *userRepository) many(ctx context.Context, q string, args ...any) ([]model.User, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]model.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func (r *userRepository) exec(ctx context.Context, q string, args ...any) error {
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *userRepository) UpdateBank(ctx context.Context, id int64, bank string) error {
	return r.exec(ctx, `UPDATE users SET bank = $2 WHERE id = $1`, id, bank)
}

func (r *userRepository) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return r.exec(ctx, `UPDATE users SET password_hash = $2, token_version = token_version + 1 WHERE id = $1`, id, hash)
}

func (r *userRepository) SetLocked(ctx context.Context, id int64, locked int) error {
	return r.exec(ctx, `UPDATE users SET locked = $2, token_version = token_version + 1 WHERE id = $1`, id, locked)
}

func (r *userRepository) Anonymize(ctx context.Context, id int64, hash string) error {
	return r.exec(ctx, `
		UPDATE users SET name = $2, username = $3, email = NULL, google_sub = NULL, bank = '', password_hash = $4,
			locked = $5, token_version = token_version + 1
		WHERE id = $1`,
		id, fmt.Sprintf("CTV đã xoá #%d", id), fmt.Sprintf("xoa-%d", id), hash, model.LockedDeleted)
}

func (r *userRepository) ListStaff(ctx context.Context) ([]model.User, error) {
	return r.many(ctx, `SELECT `+userCols+` FROM users WHERE role IN ('admin', 'ketoan', 'sale') ORDER BY role, name`)
}

// Tiền thưởng/trừ: đã trả (nằm trong phiếu chi chưa huỷ) và chờ trả (chưa vào phiếu nào).
const (
	adjPaidSQL = `COALESCE((SELECT SUM(a.amount) FROM adjustments a JOIN batches b ON b.id = a.batch_id
		WHERE a.ctv_id = u.id AND b.status = 'da_chi'), 0)`
	adjPendSQL = `COALESCE((SELECT SUM(a.amount) FROM adjustments a WHERE a.ctv_id = u.id AND a.batch_id IS NULL), 0)`
)

func (r *userRepository) ListCTVs(ctx context.Context, q string, limit, offset int) ([]CTVRow, int, error) {
	const where = `u.role = 'ctv' AND ($1 = '' OR u.name ILIKE '%' || $1 || '%' OR COALESCE(u.email, '') ILIKE '%' || $1 || '%'
		OR u.username LIKE '%' || $2 || '%')`
	digits := spacesRE.ReplaceAllString(q, "")
	if digits == "" {
		digits = q
	}
	var found int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE `+where, q, digits).Scan(&found); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.username, u.name, COALESCE(u.email, ''), u.locked, u.created_at, COUNT(h.id),
			COUNT(h.id) FILTER (WHERE h.status = 'cho_duyet'), COUNT(h.id) FILTER (WHERE h.status = 'da_duyet'),
			COALESCE(SUM(h.amount) FILTER (WHERE h.status = 'da_chi'), 0) + `+adjPaidSQL+`,
			COALESCE(SUM(h.amount) FILTER (WHERE h.status = 'da_duyet'), 0) + `+adjPendSQL+`,
			MAX(h.created_at)
		FROM users u LEFT JOIN houses h ON h.ctv_id = u.id
		WHERE `+where+`
		GROUP BY u.id
		ORDER BY COALESCE(MAX(h.id), 0) DESC, u.id DESC
		LIMIT $3 OFFSET $4`, q, digits, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]CTVRow, 0)
	for rows.Next() {
		var c CTVRow
		if err := rows.Scan(&c.ID, &c.Username, &c.Name, &c.Email, &c.Locked, &c.CreatedAt, &c.Houses, &c.Waiting,
			&c.Approved, &c.Paid, &c.Owed, &c.LastAt); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, found, rows.Err()
}

func (r *userRepository) CountCTVs(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'ctv' AND locked < 2`).Scan(&n)
	return n, err
}

func (r *userRepository) CountNewCTVs(ctx context.Context, from, to time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'ctv' AND created_at >= $1 AND created_at < $2`,
		from, to).Scan(&n)
	return n, err
}
