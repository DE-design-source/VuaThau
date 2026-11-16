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
