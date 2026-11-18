package service

import (
	"context"
	"math"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/repository"
)

// Công trình = các hồ sơ (đã gửi, không bị từ chối) cách nhau trong bán kính chặn trùng (DupRadiusM).
//   - LastAt: lần chụp gần nhất ở công trình đó. Quá DupDays ngày (mặc định 30) là "cần cập nhật công trình"
//     (qua hạn chặn trùng, CTV được chụp lại).
//   - VIP: hồ sơ chụp công trình đó đầu tiên (không có hồ sơ nào gần đó gửi sớm hơn).

type SiteInfo struct {
	LastAt time.Time
	LastID int64 // hồ sơ chụp gần nhất của công trình (lấy hình đại diện)
	VIP    bool
}

// siteInfos: mọi hồ sơ có toạ độ → thông tin công trình. Chia lưới ô cỡ bán kính, chỉ so với 9 ô quanh.
func siteInfos(ds []repository.SiteDate, radiusM float64) map[int64]SiteInfo {
	if radiusM <= 0 {
		radiusM = 50
	}
	cell := radiusM / 100000 // độ; 1° vĩ ≈ 111km nên ô lớn hơn bán kính một chút
	type key struct{ a, b int64 }
	grid := map[key][]int{}
	k := func(d repository.SiteDate) key {
		return key{int64(math.Floor(d.Lat / cell)), int64(math.Floor(d.Lng / cell))}
	}
	for i, d := range ds {
		grid[k(d)] = append(grid[k(d)], i)
	}
	earlier := func(a, b repository.SiteDate) bool { return a.At.Before(b.At) || (a.At.Equal(b.At) && a.ID < b.ID) }
	out := make(map[int64]SiteInfo, len(ds))
	for _, d := range ds {
		info := SiteInfo{LastAt: d.At, LastID: d.ID, VIP: true}
		c := k(d)
		for da := int64(-1); da <= 1; da++ {
			for db := int64(-1); db <= 1; db++ {
				for _, j := range grid[key{c.a + da, c.b + db}] {
					o := ds[j]
					if o.ID == d.ID || imaging.Meters(d.Lat, d.Lng, o.Lat, o.Lng) > radiusM {
						continue
					}
					if o.At.After(info.LastAt) || (o.At.Equal(info.LastAt) && o.ID > info.LastID) {
						info.LastAt, info.LastID = o.At, o.ID
					}
					if earlier(o, d) {
						info.VIP = false
					}
				}
			}
		}
		out[d.ID] = info
	}
	return out
}

// sites: thông tin công trình của mọi hồ sơ + số ngày coi là cần cập nhật.
func (s *houseService) sites(ctx context.Context) (map[int64]SiteInfo, int, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, 0, err
	}
	ds, err := s.houses.SiteDates(ctx)
	if err != nil {
		return nil, 0, err
	}
	return siteInfos(ds, float64(st.DupRadiusM)), st.DupDays, nil
}

// markVIP: gắn nhãn VIP (chụp công trình đầu tiên) cho các hồ sơ.
func (s *houseService) markVIP(ctx context.Context, rows []model.House) error {
	if len(rows) == 0 {
		return nil
	}
	idx, _, err := s.sites(ctx)
	if err != nil {
		return err
	}
	for i := range rows {
		rows[i].VIP = idx[rows[i].ID].VIP
	}
	return nil
}

// siteCovers: hình đại diện (ảnh đầu của hồ sơ chụp gần nhất) cho công trình chứa mỗi hồ sơ trong ids.
func (s *houseService) siteCovers(ctx context.Context, idx map[int64]SiteInfo, ids []int64) (map[int64]int64, error) {
	last := make([]int64, 0, len(ids))
	for _, id := range ids {
		if in, ok := idx[id]; ok {
			last = append(last, in.LastID)
		} else {
			last = append(last, id)
		}
	}
	covers, err := s.houses.Covers(ctx, last)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int64, len(ids))
	for i, id := range ids {
		if c, ok := covers[last[i]]; ok {
			out[id] = c
		}
	}
	return out, nil
}

// JournalSet: 1 bộ ảnh (1 hồ sơ) trong nhật ký công trình.
type JournalSet struct {
	HouseID int64   `json:"house_id"`
	Stage   string  `json:"stage"`
	Status  string  `json:"status"`
	At      string  `json:"at"`
	CTV     string  `json:"ctv,omitempty"` // tên người chụp (không gửi cho CTV)
	Photos  []int64 `json:"photos"`
	Self    bool    `json:"self,omitempty"` // chính hồ sơ đang xem
}

// journal: các bộ ảnh đã duyệt của cùng công trình với h (trong bán kính chặn trùng) + chính h, cũ trước.
// CTV chỉ thấy hồ sơ của mình (ảnh hồ sơ người khác CTV không được mở).
func (s *houseService) journal(ctx context.Context, u *model.User, h *model.House, radiusM int) ([]JournalSet, error) {
	if h.Lat == nil || h.Lng == nil {
		ps, err := s.photos.ListByHouse(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		at := h.CreatedAt
		if h.SubmittedAt != nil {
			at = *h.SubmittedAt
		}
		return []JournalSet{{HouseID: h.ID, Stage: h.Stage, Status: h.Status, At: at.In(VN).Format(time.RFC3339), Photos: photoIDs(ps), Self: true}}, nil
	}
	r := float64(radiusM)
	if r <= 0 {
		r = 50
	}
	var ctv int64
	if u.Role == model.RoleCTV {
		ctv = u.ID
	}
	rows, err := s.houses.Journal(ctx, *h.Lat, *h.Lng, r/100000*1.5, h.ID, ctv)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	near := rows[:0]
	for _, j := range rows {
		if j.ID == h.ID || imaging.Meters(*h.Lat, *h.Lng, j.Lat, j.Lng) <= r {
			near = append(near, j)
			ids = append(ids, j.ID)
		}
	}
	ps, err := s.photos.ListByHouses(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]JournalSet, 0, len(near))
	for _, j := range near {
		set := JournalSet{HouseID: j.ID, Stage: j.Stage, Status: j.Status, At: j.At.In(VN).Format(time.RFC3339), Photos: photoIDs(ps[j.ID]), Self: j.ID == h.ID}
		if u.Role != model.RoleCTV {
			set.CTV = j.CTVName
		}
		if len(set.Photos) > 0 || set.Self {
			out = append(out, set)
		}
	}
	return out, nil
}

func photoIDs(ps []model.Photo) []int64 {
	out := make([]int64, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}
