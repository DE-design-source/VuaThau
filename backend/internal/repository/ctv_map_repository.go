package repository

import (
	"context"
	"time"
)

// OtherSite: công trình CTV khác đã chụp, chỉ toạ độ (bản đồ trong app CTV, để tránh chụp trùng).
// KHÔNG kèm mã, địa chỉ, nhà thầu, ảnh, tên CTV.
type OtherSite struct {
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Recent bool    `json:"recent"` // gửi trong số ngày chặn trùng: chụp lại chỗ này sẽ bị chặn
}

// OtherSites: công trình (đã gửi, không bị từ chối) của CTV khác ctvID.
func (r *houseRepository) OtherSites(ctx context.Context, ctvID int64, since time.Time) ([]OtherSite, error) {
	rows, err := r.db.Query(ctx, `SELECT h.lat, h.lng, COALESCE(h.submitted_at, h.created_at) >= $2 FROM houses h
		WHERE h.ctv_id <> $1 AND `+saleVisible+` AND h.lat IS NOT NULL AND h.lng IS NOT NULL ORDER BY h.id DESC LIMIT 20000`, ctvID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OtherSite{}
	for rows.Next() {
		var s OtherSite
		if err := rows.Scan(&s.Lat, &s.Lng, &s.Recent); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
