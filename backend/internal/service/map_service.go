package service

import (
	"context"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// MapData: bản đồ công trình toàn quốc + bảng số công trình mỗi CTV đã chụp + theo 34 tỉnh, thành.
type MapData struct {
	Points    []repository.MapPoint `json:"points"`
	CTVs      []repository.CTVSites `json:"ctvs"`
	CTVPins   []CTVPin              `json:"ctv_pins"`  // lớp Cộng tác viên: mọi CTV, vị trí, loại, trạng thái
	Customers []repository.Customer `json:"customers"` // lớp Khách hàng (khách của CTV kinh doanh)
	Provinces []ProvinceSites       `json:"provinces"` // đủ 34 tỉnh (kể cả chưa có công trình), theo bộ lọc CTV / ngày
	DupDays   int                   `json:"dup_days"`
	Filter    OverviewQuery         `json:"filter"`
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
	since := s.now().AddDate(0, 0, -st.DupDays)
	res := &MapData{DupDays: st.DupDays, Filter: q}
	// Số theo tỉnh: theo CTV / ngày / trạng thái, chưa lọc tỉnh (để thấy cả nước)
	counts, err := s.houses.ProvinceSites(ctx, f, since)
	if err != nil {
		return nil, err
	}
	for _, p := range province.All() {
		c := counts[p.Code]
		res.Provinces = append(res.Provinces, ProvinceSites{Code: p.Code, Name: p.Name, Merged: p.Merged, Total: c[0], Recent: c[1]})
	}
	if province.ByCode(q.Province) != nil {
		f.Province = q.Province
	} else {
		q.Province = ""
		res.Filter.Province = ""
	}
	if res.Points, err = s.houses.MapPoints(ctx, f); err != nil {
		return nil, err
	}
	// Màu theo lần chụp gần nhất của công trình; VIP: chụp công trình đầu tiên
	idx, _, err := s.sites(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(res.Points))
	for i, p := range res.Points {
		ids[i] = p.ID
	}
	covers, err := s.siteCovers(ctx, idx, ids)
	if err != nil {
		return nil, err
	}
	for i := range res.Points {
		if c, ok := covers[res.Points[i].ID]; ok {
			res.Points[i].CoverID = ptr(c)
		}
		in, ok := idx[res.Points[i].ID]
		res.Points[i].LastAt, res.Points[i].VIP = res.Points[i].At, ok && in.VIP
		if ok {
			res.Points[i].LastAt = in.LastAt
		}
	}
	if res.CTVs, err = s.houses.SiteCounts(ctx, since); err != nil {
		return nil, err
	}
	if res.CTVPins, err = s.ctvPins(ctx, st.DupDays); err != nil {
		return nil, err
	}
	res.Customers = []repository.Customer{}
	if s.customers != nil {
		if res.Customers, err = s.customers.List(ctx, 0); err != nil {
			return nil, err
		}
	}
	return res, nil
}
