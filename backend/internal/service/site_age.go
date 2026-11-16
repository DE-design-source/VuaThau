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
