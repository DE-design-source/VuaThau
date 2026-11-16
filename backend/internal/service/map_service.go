package service

import (
	"context"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// MapData: bản đồ công trình toàn quốc + bảng số công trình mỗi CTV đã chụp.
type MapData struct {
	Points  []repository.MapPoint `json:"points"`
	CTVs    []repository.CTVSites `json:"ctvs"`
	DupDays int                   `json:"dup_days"`
	Filter  OverviewQuery         `json:"filter"`
}

func (s *houseService) Map(ctx context.Context, q OverviewQuery) (*MapData, error) {
	f := repository.HouseFilter{CTVID: q.CTV}
	if _, ok := model.Statuses[q.Status]; ok {
		f.Statuses = []string{q.Status}
	} else {
		q.Status = ""
	}
	if t := dayStart(q.From); t != nil {
		f.From = t
	} else {
		q.From = ""
	}
	if t := dayStart(q.To); t != nil {
		f.To = ptr(t.AddDate(0, 0, 1))
	} else {
		q.To = ""
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	res := &MapData{DupDays: st.DupDays, Filter: q}
	if res.Points, err = s.houses.MapPoints(ctx, f); err != nil {
		return nil, err
	}
	if res.CTVs, err = s.houses.SiteCounts(ctx, s.now().AddDate(0, 0, -st.DupDays)); err != nil {
		return nil, err
	}
	return res, nil
}
