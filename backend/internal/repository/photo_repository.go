package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

// DHashRow: dHash 1 ảnh đã nộp (để tìm ảnh gần giống).
type DHashRow struct {
	HouseID int64
	DHash   string
}

type PhotoRepository interface {
	Insert(ctx context.Context, p *model.Photo) error
	// HouseOfSHA: hồ sơ đã có ảnh trùng hệt (sha256), 0 nếu chưa có.
	HouseOfSHA(ctx context.Context, sha string) (int64, error)
	AllDHashes(ctx context.Context) ([]DHashRow, error)
	ListByHouse(ctx context.Context, houseID int64) ([]model.Photo, error)
	ListByHouses(ctx context.Context, houseIDs []int64) (map[int64][]model.Photo, error)
	Get(ctx context.Context, id int64) (*model.Photo, error)
	Delete(ctx context.Context, id, houseID int64) (*model.Photo, error)
	Count(ctx context.Context, houseID int64) (int, error)
	// ApplyAI ghi kết quả AI đọc 1 ảnh (giờ in trên ảnh, địa chỉ, có phải công trình, toạ độ nếu còn trống).
	ApplyAI(ctx context.Context, id int64, aiTime, addr string, isSite bool, lat, lng *float64) error

	CacheGet(ctx context.Context, shas []string) (map[string]json.RawMessage, error)
	CachePut(ctx context.Context, sha string, data json.RawMessage) error
}

type photoRepository struct {
	db *pgxpool.Pool
}

func NewPhotoRepository(db *pgxpool.Pool) PhotoRepository {
	return &photoRepository{db: db}
}

const photoCols = `id, house_id, sha256, dhash, exif_time, ai_time, lat, lng, address, is_site, dup_of, mime`

func scanPhoto(row pgx.Row) (*model.Photo, error) {
	var p model.Photo
	err := row.Scan(&p.ID, &p.HouseID, &p.SHA256, &p.DHash, &p.ExifTime, &p.AITime, &p.Lat, &p.Lng, &p.Address,
		&p.IsSite, &p.DupOf, &p.Mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (r *photoRepository) Insert(ctx context.Context, p *model.Photo) error {
	err := r.db.QueryRow(ctx, `INSERT INTO photos (house_id, sha256, dhash, exif_time, lat, lng, address, dup_of, mime)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		p.HouseID, p.SHA256, p.DHash, p.ExifTime, p.Lat, p.Lng, p.Address, p.DupOf, p.Mime).Scan(&p.ID)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	return err
}

func (r *photoRepository) HouseOfSHA(ctx context.Context, sha string) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `SELECT house_id FROM photos WHERE sha256 = $1`, sha).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (r *photoRepository) AllDHashes(ctx context.Context) ([]DHashRow, error) {
	rows, err := r.db.Query(ctx, `SELECT house_id, dhash FROM photos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DHashRow{}
	for rows.Next() {
		var d DHashRow
		if err := rows.Scan(&d.HouseID, &d.DHash); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *photoRepository) list(ctx context.Context, q string, args ...any) ([]model.Photo, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *photoRepository) ListByHouse(ctx context.Context, houseID int64) ([]model.Photo, error) {
	return r.list(ctx, `SELECT `+photoCols+` FROM photos WHERE house_id = $1 ORDER BY id`, houseID)
}

func (r *photoRepository) ListByHouses(ctx context.Context, ids []int64) (map[int64][]model.Photo, error) {
	out := map[int64][]model.Photo{}
	if len(ids) == 0 {
		return out, nil
	}
	ps, err := r.list(ctx, `SELECT `+photoCols+` FROM photos WHERE house_id = ANY($1) ORDER BY id`, ids)
	for _, p := range ps {
		out[p.HouseID] = append(out[p.HouseID], p)
	}
	return out, err
}

func (r *photoRepository) Get(ctx context.Context, id int64) (*model.Photo, error) {
	return scanPhoto(r.db.QueryRow(ctx, `SELECT `+photoCols+` FROM photos WHERE id = $1`, id))
}

func (r *photoRepository) Delete(ctx context.Context, id, houseID int64) (*model.Photo, error) {
	return scanPhoto(r.db.QueryRow(ctx, `DELETE FROM photos WHERE id = $1 AND house_id = $2 RETURNING `+photoCols, id, houseID))
}

func (r *photoRepository) Count(ctx context.Context, houseID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM photos WHERE house_id = $1`, houseID).Scan(&n)
	return n, err
}

func (r *photoRepository) ApplyAI(ctx context.Context, id int64, aiTime, addr string, isSite bool, lat, lng *float64) error {
	_, err := r.db.Exec(ctx, `UPDATE photos SET ai_time = $2, address = COALESCE(NULLIF($3, ''), address), is_site = $4,
		lat = COALESCE(lat, $5), lng = COALESCE(lng, $6) WHERE id = $1`, id, aiTime, addr, isSite, lat, lng)
	return err
}

func (r *photoRepository) CacheGet(ctx context.Context, shas []string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(shas) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT sha256, data FROM ai_cache WHERE sha256 = ANY($1)`, shas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var d []byte
		if err := rows.Scan(&s, &d); err != nil {
			return nil, err
		}
		out[s] = d
	}
	return out, rows.Err()
}

func (r *photoRepository) CachePut(ctx context.Context, sha string, data json.RawMessage) error {
	_, err := r.db.Exec(ctx, `INSERT INTO ai_cache (sha256, data) VALUES ($1, $2)
		ON CONFLICT (sha256) DO UPDATE SET data = EXCLUDED.data, at = NOW()`, sha, data)
	return err
}
