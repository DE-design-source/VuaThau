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
	res := &CTVMap{Mine: []CTVMapPoint{}, DupDays: st.DupDays}
	for _, p := range pts {
		res.Mine = append(res.Mine, CTVMapPoint{ID: p.ID, Lat: p.Lat, Lng: p.Lng, Status: p.Status, Address: p.Address,
			At: p.At.In(VN).Format("02/01/2006")})
	}
	if res.Others, err = s.houses.OtherSites(ctx, u.ID, s.now().AddDate(0, 0, -st.DupDays)); err != nil {
		return nil, err
	}
	return res, nil
}
