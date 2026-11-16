package repository

import (
	"context"
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

var spacesRE = regexp.MustCompile(`[\s.\-]`)

type AuthRepository interface {
	CreateUser(ctx context.Context, u *model.User) error
	// FindByLogin: theo tên đăng nhập (SĐT của CTV, SĐT hoặc tên của nhân viên).
	FindByLogin(ctx context.Context, login string) (*model.User, error)
	FindByGoogle(ctx context.Context, sub string) (*model.User, error)
	LinkGoogle(ctx context.Context, id int64, sub, email string) error
}

type authRepository struct {
	db *pgxpool.Pool
}

func NewAuthRepository(db *pgxpool.Pool) AuthRepository {
	return &authRepository{db: db}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *authRepository) CreateUser(ctx context.Context, u *model.User) error {
	if u.Role == "" {
		u.Role = model.RoleCTV
	}
	const q = `
		INSERT INTO users (username, email, password_hash, name, role, bank, google_sub)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, locked, token_version, created_at, updated_at`

	err := r.db.QueryRow(ctx, q, u.Username, nullIfEmpty(u.Email), u.PasswordHash, u.Name, u.Role, u.Bank,
		nullIfEmpty(u.GoogleSub)).Scan(&u.ID, &u.Locked, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	return err
}

func (r *authRepository) FindByLogin(ctx context.Context, login string) (*model.User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE username = $1`, login))
}

func (r *authRepository) FindByGoogle(ctx context.Context, sub string) (*model.User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE google_sub = $1`, sub))
}

// RehashPassword: nâng hash mật khẩu cũ (scrypt) lên bcrypt sau khi đăng nhập đúng; không đổi token_version.
func (r *authRepository) RehashPassword(ctx context.Context, id int64, hash string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, hash)
	return err
}

func (r *authRepository) LinkGoogle(ctx context.Context, id int64, sub, email string) error {
	tag, err := r.db.Exec(ctx, `UPDATE users SET google_sub = $2, email = $3 WHERE id = $1`, id, sub, nullIfEmpty(email))
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
