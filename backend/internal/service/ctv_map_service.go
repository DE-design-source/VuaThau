package service

import (
	"context"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// CTVMapPoint: công trình của chính CTV trên bản đồ app.
type CTVMapPoint struct {
	ID      int64   `json:"id"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Status  string  `json:"status"`
	Address string  `json:"address"`
	At      string  `json:"at"`
	Recent  bool    `json:"recent"` // công trình được chụp trong DupDays ngày gần đây (chưa cần cập nhật)
	VIP     bool    `json:"vip"`
}

// CTVMap: bản đồ trong app CTV. Công trình của mình: đủ thông tin; của CTV khác: chỉ chấm (toạ độ).
type CTVMap struct {
	Mine    []CTVMapPoint          `json:"mine"`
	Others  []repository.OtherSite `json:"others"`
	DupDays int                    `json:"dup_days"`
}

func (s *houseService) CTVMap(ctx context.Context, u *model.User) (*CTVMap, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	pts, err := s.houses.MapPoints(ctx, repository.HouseFilter{CTVID: u.ID})
	if err != nil {
		return nil, err
	}
	idx, _, err := s.sites(ctx)
	if err != nil {
		return nil, err
	}
	since := s.now().AddDate(0, 0, -st.DupDays)
	res := &CTVMap{Mine: []CTVMapPoint{}, DupDays: st.DupDays}
	for _, p := range pts {
		last := p.At
		in, ok := idx[p.ID]
		if ok {
			last = in.LastAt
		}
		res.Mine = append(res.Mine, CTVMapPoint{ID: p.ID, Lat: p.Lat, Lng: p.Lng, Status: p.Status, Address: p.Address,
			At: p.At.In(VN).Format("02/01/2006"), Recent: !last.Before(since), VIP: ok && in.VIP})
	}
	if res.Others, err = s.houses.OtherSites(ctx, u.ID, since); err != nil {
		return nil, err
	}
	// mới / cần cập nhật: theo lần chụp gần nhất của cả công trình (giống bản đồ web)
	for i := range res.Others {
		if in, ok := idx[res.Others[i].ID]; ok {
			res.Others[i].Recent = !in.LastAt.Before(since)
		}
	}
	return res, nil
}
