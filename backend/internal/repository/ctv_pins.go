package repository

import (
	"context"
	"time"
)

// CTVPinRow: 1 CTV cho lớp Cộng tác viên trên bản đồ admin / kế toán (góp ý 21/11/2026).
type CTVPinRow struct {
	ID       int64
	Name     string
	Phone    string
	Kind     string     // chức năng (admin đặt): "CTV", "CTV kinh doanh"...
	Region   string     // mã tỉnh admin đặt (VN79...) hoặc ''
	Addr     string     // địa chỉ bên Messenger (dia_chi)
	Area     string     // khu vực bên Messenger (khu_vuc)
	Total    int        // hồ sơ đã gửi (không tính nháp, từ chối)
	Last     *time.Time // lần gửi gần nhất
	HouseLat *float64   // công trình gần nhất CTV chụp (có toạ độ)
	HouseLng *float64
	HouseAt  string // địa chỉ công trình đó
}

// CTVPins: mọi CTV chưa xoá, kèm công trình gần nhất có toạ độ.
func (r *houseRepository) CTVPins(ctx context.Context) ([]CTVPinRow, error) {
	rows, err := r.db.Query(ctx, `SELECT u.id, u.name, u.username, COALESCE(u.kind, ''), COALESCE(u.region, ''),
			COALESCE(u.inbox_info->'fields'->>'dia_chi', ''), COALESCE(u.inbox_info->'fields'->>'khu_vuc', ''),
			(SELECT COUNT(*) FROM houses h WHERE h.ctv_id = u.id AND h.status NOT IN ('nhap', 'tu_choi')),
			(SELECT MAX(h.submitted_at) FROM houses h WHERE h.ctv_id = u.id AND h.status NOT IN ('nhap', 'tu_choi')),
			lh.lat, lh.lng, COALESCE(lh.address, '')
		FROM users u
		LEFT JOIN LATERAL (SELECT h.lat, h.lng, h.address FROM houses h
			WHERE h.ctv_id = u.id AND h.status NOT IN ('nhap', 'tu_choi') AND h.lat IS NOT NULL AND h.lng IS NOT NULL
			ORDER BY h.submitted_at DESC NULLS LAST, h.id DESC LIMIT 1) lh ON TRUE
		WHERE u.role = 'ctv' AND u.locked <> 2
		ORDER BY 8 DESC, u.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CTVPinRow{}
	for rows.Next() {
		var c CTVPinRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Kind, &c.Region, &c.Addr, &c.Area, &c.Total, &c.Last, &c.HouseLat, &c.HouseLng, &c.HouseAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
