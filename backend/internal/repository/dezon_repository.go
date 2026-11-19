package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DezonProject: 1 công trình nhà thầu đăng trên dezon.vn (bảng dezon_projects), lớp ghim riêng trên bản đồ.
type DezonProject struct {
	ID            int64      `json:"id"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Contractor    string     `json:"contractor"`
	ContractorURL string     `json:"contractor_url"`
	Cover         string     `json:"cover"`
	PostedAt      *time.Time `json:"posted_at"`
	Lat           *float64   `json:"lat"`
	Lng           *float64   `json:"lng"`
	Province      string     `json:"province"`
	GeoSrc        string     `json:"geo_src"`   // google | osm
	GeoLevel      string     `json:"geo_level"` // dia_chi | khu_vuc
}

// DezonUpsert: dữ liệu đọc từ thẻ công trình.
type DezonUpsert struct {
	URL, Title, Contractor, ContractorURL, Cover string
	PostedAt                                     time.Time
}

// DezonGeo: kết quả tra toạ độ (Lat/Lng nil: không tra được).
type DezonGeo struct {
	Lat, Lng                    *float64
	Province, Src, Level, Query string
}

type DezonRepository interface {
	// Upsert: thêm công trình mới (ngày đăng chỉ ghi lần đầu), cập nhật tiêu đề / ảnh / nhà thầu. Trả true nếu mới.
	Upsert(ctx context.Context, p DezonUpsert) (bool, error)
	// NeedGeo: công trình chưa tra toạ độ, tra lỗi quá retry trước, hoặc (google) chưa tra bằng Google.
	NeedGeo(ctx context.Context, google bool, retry time.Duration, limit int) ([]DezonProject, error)
	SetGeo(ctx context.Context, id int64, g DezonGeo) error
	// Points: công trình đã có toạ độ (province trống: cả nước).
	Points(ctx context.Context, province string) ([]DezonProject, error)
}

type dezonRepository struct{ db *pgxpool.Pool }

func NewDezonRepository(db *pgxpool.Pool) DezonRepository { return &dezonRepository{db: db} }

const dezonCols = `id, url, title, contractor, contractor_url, cover, posted_at, lat, lng, province, geo_src, geo_level`

func (r *dezonRepository) Upsert(ctx context.Context, p DezonUpsert) (bool, error) {
	var inserted bool
	err := r.db.QueryRow(ctx, `INSERT INTO dezon_projects (url, title, contractor, contractor_url, cover, posted_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url) DO UPDATE SET
			-- đổi tiêu đề (địa chỉ): tra lại toạ độ
			geo_at = CASE WHEN dezon_projects.title <> EXCLUDED.title THEN NULL ELSE dezon_projects.geo_at END,
			title = EXCLUDED.title, contractor = EXCLUDED.contractor, contractor_url = EXCLUDED.contractor_url,
			cover = EXCLUDED.cover, last_seen = NOW()
		RETURNING xmax = 0`, p.URL, p.Title, p.Contractor, p.ContractorURL, p.Cover, p.PostedAt).Scan(&inserted)
	return inserted, err
}

func (r *dezonRepository) NeedGeo(ctx context.Context, google bool, retry time.Duration, limit int) ([]DezonProject, error) {
	return r.list(ctx, `WHERE geo_at IS NULL OR (lat IS NULL AND geo_at < $1) OR ($2 AND geo_src <> 'google')
		ORDER BY geo_at NULLS FIRST, id LIMIT $3`, time.Now().Add(-retry), google, limit)
}

func (r *dezonRepository) SetGeo(ctx context.Context, id int64, g DezonGeo) error {
	_, err := r.db.Exec(ctx, `UPDATE dezon_projects SET lat = $2, lng = $3, province = $4, geo_src = $5, geo_level = $6,
		geo_query = $7, geo_at = NOW() WHERE id = $1`, id, g.Lat, g.Lng, g.Province, g.Src, g.Level, g.Query)
	return err
}

func (r *dezonRepository) Points(ctx context.Context, province string) ([]DezonProject, error) {
	return r.list(ctx, `WHERE lat IS NOT NULL AND lng IS NOT NULL AND ($1 = '' OR province = $1)
		ORDER BY posted_at DESC NULLS LAST, id DESC LIMIT 5000`, province)
}

func (r *dezonRepository) list(ctx context.Context, where string, args ...any) ([]DezonProject, error) {
	rows, err := r.db.Query(ctx, `SELECT `+dezonCols+` FROM dezon_projects `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DezonProject{}
	for rows.Next() {
		var p DezonProject
		if err := rows.Scan(&p.ID, &p.URL, &p.Title, &p.Contractor, &p.ContractorURL, &p.Cover, &p.PostedAt,
			&p.Lat, &p.Lng, &p.Province, &p.GeoSrc, &p.GeoLevel); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
