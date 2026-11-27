package repository

import (
	"context"
	"time"
)

// SiteDate: 1 hồ sơ thật (đã gửi, không bị từ chối) có toạ độ: để gom thành công trình (các hồ sơ trong bán kính
// chặn trùng là cùng 1 công trình), tìm lần chụp gần nhất và người chụp đầu tiên (VIP).
type SiteDate struct {
	ID       int64
	Lat, Lng float64
	At       time.Time
}

func (r *houseRepository) SiteDates(ctx context.Context) ([]SiteDate, error) {
	rows, err := r.db.Query(ctx, `SELECT h.id, h.lat, h.lng, COALESCE(h.submitted_at, h.created_at) FROM houses h
		WHERE `+saleVisible+` AND h.lat IS NOT NULL AND h.lng IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SiteDate{}
	for rows.Next() {
		var s SiteDate
		if err := rows.Scan(&s.ID, &s.Lat, &s.Lng, &s.At); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Covers: ảnh đại diện của mỗi hồ sơ (hồ sơ id -> ảnh id): ảnh AI chấm điểm đại diện cao nhất; bằng nhau / chưa chấm thì ảnh đầu.
func (r *houseRepository) Covers(ctx context.Context, ids []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT DISTINCT ON (house_id) house_id, id FROM photos WHERE house_id = ANY($1)
		ORDER BY house_id, cover_score DESC, id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h, p int64
		if err := rows.Scan(&h, &p); err != nil {
			return nil, err
		}
		out[h] = p
	}
	return out, rows.Err()
}

// SiteHit: công trình khớp ô tìm trên bản đồ (tên / SĐT nhà thầu, địa chỉ).
type SiteHit struct {
	ID         int64   `json:"id"`
	Contractor string  `json:"contractor"`
	Address    string  `json:"address"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
}

// SearchSites: tìm công trình (đã gửi, không bị từ chối, có toạ độ) theo tên / SĐT nhà thầu, địa chỉ, mã.
// ctvID > 0: chỉ công trình của CTV đó (CTV không được xem chi tiết công trình người khác).
func (r *houseRepository) SearchSites(ctx context.Context, q string, ctvID int64) ([]SiteHit, error) {
	where, args := SaleFilter{Q: q, CTVID: ctvID}.where()
	rows, err := r.db.Query(ctx, `SELECT h.id, h.contractor_name, h.address, h.lat, h.lng FROM houses h WHERE `+where+
		` AND h.lat IS NOT NULL AND h.lng IS NOT NULL ORDER BY COALESCE(h.submitted_at, h.created_at) DESC LIMIT 6`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SiteHit{}
	for rows.Next() {
		var s SiteHit
		if err := rows.Scan(&s.ID, &s.Contractor, &s.Address, &s.Lat, &s.Lng); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// JournalHouse: 1 hồ sơ ở quanh 1 toạ độ (nhật ký ảnh công trình theo giai đoạn).
type JournalHouse struct {
	ID       int64
	Lat, Lng float64
	Stage    string
	Status   string
	CTVName  string
	At       time.Time
}

// Journal: hồ sơ đã duyệt / đã thanh toán (và chính hồ sơ self) trong ô vuông ±deg độ quanh (lat, lng), cũ trước.
// ctvID > 0: chỉ hồ sơ của CTV đó.
func (r *houseRepository) Journal(ctx context.Context, lat, lng, deg float64, self, ctvID int64) ([]JournalHouse, error) {
	rows, err := r.db.Query(ctx, `SELECT h.id, h.lat, h.lng, h.stage, h.status, COALESCE(u.name, ''), COALESCE(h.submitted_at, h.created_at)
		FROM houses h LEFT JOIN users u ON u.id = h.ctv_id
		WHERE h.lat BETWEEN $1::float8 - $3::float8 AND $1::float8 + $3::float8 AND h.lng BETWEEN $2::float8 - $3::float8 AND $2::float8 + $3::float8
		  AND (h.status IN ('da_duyet', 'da_chi') OR h.id = $4::bigint) AND ($5::bigint = 0 OR h.ctv_id = $5::bigint)
		ORDER BY 7, 1`, lat, lng, deg, self, ctvID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JournalHouse{}
	for rows.Next() {
		var j JournalHouse
		if err := rows.Scan(&j.ID, &j.Lat, &j.Lng, &j.Stage, &j.Status, &j.CTVName, &j.At); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
