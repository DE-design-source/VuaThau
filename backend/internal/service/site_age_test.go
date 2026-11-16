package service

import (
	"testing"
	"time"

	"vuathau/backend/internal/repository"
)

func TestSiteInfos(t *testing.T) {
	d0 := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	ds := []repository.SiteDate{
		{ID: 1, Lat: 10.800000, Lng: 106.700000, At: d0},                   // chụp đầu tiên
		{ID: 2, Lat: 10.800200, Lng: 106.700100, At: d0.AddDate(0, 1, 5)},  // cách ~25m: cùng công trình, chụp lại sau
		{ID: 3, Lat: 10.810000, Lng: 106.700000, At: d0.AddDate(0, 0, 10)}, // cách ~1,1km: công trình khác
		{ID: 4, Lat: 10.810000, Lng: 106.700000, At: d0.AddDate(0, 0, 10)}, // cùng chỗ, cùng giờ với 3: id nhỏ hơn là VIP
		{ID: 5, Lat: 10.800000, Lng: 106.700800, At: d0.AddDate(0, 0, -3)}, // cách #1 ~87m, #2 ~79m: ngoài bán kính 50m
	}
	got := siteInfos(ds, 50)
	want := map[int64]SiteInfo{
		1: {LastAt: d0.AddDate(0, 1, 5), VIP: true},
		2: {LastAt: d0.AddDate(0, 1, 5), VIP: false},
		3: {LastAt: d0.AddDate(0, 0, 10), VIP: true},
		4: {LastAt: d0.AddDate(0, 0, 10), VIP: false},
		5: {LastAt: d0.AddDate(0, 0, -3), VIP: true},
	}
	// hình đại diện lấy từ hồ sơ chụp gần nhất của công trình: #1 và #2 cùng công trình -> #2; #3, #4 cùng giờ -> id lớn hơn
	for id, last := range map[int64]int64{1: 2, 2: 2, 3: 4, 4: 4, 5: 5} {
		if got[id].LastID != last {
			t.Errorf("#%d LastID = %d, muốn %d", id, got[id].LastID, last)
		}
	}
	for id, w := range want {
		if g := got[id]; !g.LastAt.Equal(w.LastAt) || g.VIP != w.VIP {
			t.Errorf("#%d: %+v, muốn %+v", id, g, w)
		}
	}
}
