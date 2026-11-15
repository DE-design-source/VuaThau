package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

type AuthRepository interface {
	CreateUser(ctx context.Context, u *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
}

type authRepository struct {
	db *pgxpool.Pool
}

func NewAuthRepository(db *pgxpool.Pool) AuthRepository {
	return &authRepository{db: db}
}

func (r *authRepository) CreateUser(ctx context.Context, u *model.User) error {
	const q = `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, q, u.Username, u.Email, u.PasswordHash).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	return err
}

func (r *authRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	const q = `
		SELECT id, username, email, password_hash, created_at, updated_at
		FROM users WHERE email = $1`

	var u model.User
	err := r.db.QueryRow(ctx, q, email).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
