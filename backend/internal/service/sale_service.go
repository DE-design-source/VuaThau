package service

import (
	"context"
	"errors"

	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// Trang sale: bản đồ, dự án, nhà thầu. Chỉ thông tin công trình, không có CTV, không có tiền.

const salePageSize = 30

// SaleMapData: công trình trên bản đồ + số công trình đủ 34 tỉnh.
type SaleMapData struct {
	Points    []repository.SalePoint `json:"points"`
	Provinces []ProvinceSites        `json:"provinces"`
	Province  string                 `json:"province"`
	CTV       int64                  `json:"ctv"`
	CTVs      []repository.SaleCTV   `json:"ctvs"`       // ô lọc CTV: chỉ tên + số công trình
	StaleDays int                    `json:"stale_days"` // lần chụp gần nhất quá bấy nhiêu ngày: cần cập nhật công trình
}

type SaleProjectList struct {
	Rows  []repository.SaleProject `json:"rows"`
	Found int                      `json:"found"`
	Page  int                      `json:"page"`
	Pages int                      `json:"pages"`
}

// SalePhoto: ảnh công trình, chỉ giờ chụp và vị trí.
type SalePhoto struct {
	ID   int64    `json:"id"`
	Time string   `json:"time"`
	Lat  *float64 `json:"lat"`
	Lng  *float64 `json:"lng"`
}

type SaleProjectDetail struct {
	*repository.SaleProject
	PhotoList []SalePhoto `json:"photo_list"`
}

func provinceName(code string) string {
	if p := province.ByCode(code); p != nil {
		return p.Name
	}
	return ""
}

func (s *houseService) SaleMap(ctx context.Context, prov string, ctv int64) (*SaleMapData, error) {
	counts, err := s.houses.SaleProvinceCounts(ctx)
	if err != nil {
		return nil, err
	}
	res := &SaleMapData{}
	for _, p := range province.All() {
		res.Provinces = append(res.Provinces, ProvinceSites{Code: p.Code, Name: p.Name, Merged: p.Merged, Total: counts[p.Code]})
	}
	if province.ByCode(prov) == nil {
		prov = ""
	}
	res.Province, res.CTV = prov, max(ctv, 0)
	if res.CTVs, err = s.houses.SaleCTVs(ctx); err != nil {
		return nil, err
	}
	if res.Points, err = s.houses.SalePoints(ctx, repository.SaleFilter{Province: prov, CTVID: res.CTV}); err != nil {
		return nil, err
	}
	idx, days, err := s.sites(ctx)
	if err != nil {
		return nil, err
	}
	res.StaleDays = days
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
		res.Points[i].LastAt = res.Points[i].At
		if in, ok := idx[res.Points[i].ID]; ok {
			res.Points[i].LastAt = in.LastAt
		}
	}
	return res, nil
}

func (s *houseService) SaleProjects(ctx context.Context, f repository.SaleFilter, page int) (*SaleProjectList, error) {
	if province.ByCode(f.Province) == nil {
		f.Province = ""
	}
	page = max(page, 1)
	rows, total, err := s.houses.SaleProjects(ctx, f, salePageSize, (page-1)*salePageSize)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].ProvinceName = provinceName(rows[i].Province)
	}
	return &SaleProjectList{Rows: rows, Found: total, Page: page, Pages: max(1, (total+salePageSize-1)/salePageSize)}, nil
}

func (s *houseService) SaleProject(ctx context.Context, id int64) (*SaleProjectDetail, error) {
	p, err := s.houses.SaleProject(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrNotFound, "Không tìm thấy dự án.")
	}
	if err != nil {
		return nil, err
	}
	p.ProvinceName = provinceName(p.Province)
	ps, err := s.photos.ListByHouse(ctx, id)
	if err != nil {
		return nil, err
	}
	sortPhotos(ps, s.photoOrder(ctx))
	d := &SaleProjectDetail{SaleProject: p, PhotoList: []SalePhoto{}}
	for _, ph := range ps {
		t := ph.ExifTime
		if t == "" {
			t = ph.AITime
		}
		d.PhotoList = append(d.PhotoList, SalePhoto{ID: ph.ID, Time: t, Lat: ph.Lat, Lng: ph.Lng})
	}
	return d, nil
}

func (s *houseService) Contractors(ctx context.Context, q string) ([]repository.Contractor, error) {
	cs, err := s.houses.Contractors(ctx, q)
	if err != nil {
		return nil, err
	}
	for i := range cs {
		names := []string{}
		for _, code := range cs[i].Provinces {
			if n := provinceName(code); n != "" {
				names = append(names, n)
			}
		}
		cs[i].Provinces = names
	}
	return cs, nil
}
