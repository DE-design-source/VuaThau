package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

type SettingsRepository interface {
	Get(ctx context.Context) (model.Settings, error)
	Save(ctx context.Context, s model.Settings) error
	// Kinds: danh sách chức năng / loại CTV (admin sửa ở Cấu hình), theo thứ tự.
	Kinds(ctx context.Context) ([]string, error)
	SaveKinds(ctx context.Context, kinds []string) error
}

type settingsRepository struct {
	db *pgxpool.Pool
}

func NewSettingsRepository(db *pgxpool.Pool) SettingsRepository {
	return &settingsRepository{db: db}
}

func (r *settingsRepository) Get(ctx context.Context) (model.Settings, error) {
	s := model.DefaultSettings
	rows, err := r.db.Query(ctx, `SELECT k, v FROM settings`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		switch k {
		case "price_min":
			s.PriceMin = v
		case "price_more":
			s.PriceMore = v
		case "min_photos":
			s.MinPhotos = int(v)
		case "pay_min":
			s.PayMin = int(v)
		case "dup_days":
			s.DupDays = int(v)
		case "dup_radius_m":
			s.DupRadiusM = int(v)
		}
	}
	return s, rows.Err()
}

func (r *settingsRepository) Save(ctx context.Context, s model.Settings) error {
	_, err := r.db.Exec(ctx, `INSERT INTO settings (k, v) VALUES
		('price_min', $1), ('price_more', $2), ('min_photos', $3), ('pay_min', $4), ('dup_days', $5), ('dup_radius_m', $6)
		ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, s.PriceMin, s.PriceMore, s.MinPhotos, s.PayMin, s.DupDays, s.DupRadiusM)
	return err
}

func (r *settingsRepository) Kinds(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT name FROM ctv_kinds ORDER BY pos, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// SaveKinds thay cả danh sách (CTV đang mang loại đã bỏ giữ nguyên chữ cũ, admin đổi lại khi cần).
func (r *settingsRepository) SaveKinds(ctx context.Context, kinds []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM ctv_kinds`); err != nil {
		return err
	}
	for i, k := range kinds {
		if _, err := tx.Exec(ctx, `INSERT INTO ctv_kinds (name, pos) VALUES ($1, $2)`, k, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
