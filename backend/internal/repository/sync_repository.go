package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncRepository: trạng thái đồng bộ (bảng sync_state), vd con trỏ hồ sơ Messenger đã lấy.
type SyncRepository interface {
	Get(ctx context.Context, k string) (string, time.Time, error)
	Set(ctx context.Context, k, v string) error
}

type syncRepository struct{ db *pgxpool.Pool }

func NewSyncRepository(db *pgxpool.Pool) SyncRepository { return &syncRepository{db: db} }

// Get: chưa có khoá thì trả "" (không lỗi).
func (r *syncRepository) Get(ctx context.Context, k string) (string, time.Time, error) {
	var v string
	var at time.Time
	err := r.db.QueryRow(ctx, `SELECT v, at FROM sync_state WHERE k = $1`, k).Scan(&v, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, nil
	}
	return v, at, err
}

func (r *syncRepository) Set(ctx context.Context, k, v string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO sync_state (k, v, at) VALUES ($1, $2, NOW())
		ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v, at = NOW()`, k, v)
	return err
}
