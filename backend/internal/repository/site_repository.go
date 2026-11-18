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

// Covers: ảnh đầu tiên của mỗi hồ sơ (hồ sơ id -> ảnh id).
func (r *houseRepository) Covers(ctx context.Context, ids []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT house_id, MIN(id) FROM photos WHERE house_id = ANY($1) GROUP BY house_id`, ids)
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
