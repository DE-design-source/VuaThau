package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"vuathau/backend/internal/model"
)

// Video công trình: cùng kho với ảnh (PhotoRepository) cho đỡ phải nối thêm phụ thuộc.

const videoCols = `id, house_id, sha256, mime, size, created_at`

func scanVideo(row pgx.Row) (*model.Video, error) {
	var v model.Video
	err := row.Scan(&v.ID, &v.HouseID, &v.SHA256, &v.Mime, &v.Size, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

func (r *photoRepository) InsertVideo(ctx context.Context, v *model.Video) error {
	err := r.db.QueryRow(ctx, `INSERT INTO videos (house_id, sha256, mime, size) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`, v.HouseID, v.SHA256, v.Mime, v.Size).Scan(&v.ID, &v.CreatedAt)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	return err
}

func (r *photoRepository) HouseOfVideo(ctx context.Context, sha string) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `SELECT house_id FROM videos WHERE sha256 = $1`, sha).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (r *photoRepository) Videos(ctx context.Context, houseID int64) ([]model.Video, error) {
	rows, err := r.db.Query(ctx, `SELECT `+videoCols+` FROM videos WHERE house_id = $1 ORDER BY id`, houseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *photoRepository) Video(ctx context.Context, id int64) (*model.Video, error) {
	return scanVideo(r.db.QueryRow(ctx, `SELECT `+videoCols+` FROM videos WHERE id = $1`, id))
}

func (r *photoRepository) DeleteVideo(ctx context.Context, id, houseID int64) (*model.Video, error) {
	return scanVideo(r.db.QueryRow(ctx, `DELETE FROM videos WHERE id = $1 AND house_id = $2 RETURNING `+videoCols, id, houseID))
}

// InboxHouses: hồ sơ Messenger (mã project_logs -> id hồ sơ).
func (r *photoRepository) InboxHouses(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT ext_id, id FROM houses WHERE source = 'inbox' AND COALESCE(ext_id, '') <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var id int64
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		out[k] = id
	}
	return out, rows.Err()
}
