package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	List(ctx context.Context, limit, offset int) ([]model.User, error)
}

type userRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	const q = `
		SELECT id, username, email, created_at, updated_at
		FROM users WHERE id = $1`

	var u model.User
	err := r.db.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) List(ctx context.Context, limit, offset int) ([]model.User, error) {
	const q = `
		SELECT id, username, email, created_at, updated_at
		FROM users ORDER BY id LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]model.User, 0, limit)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
