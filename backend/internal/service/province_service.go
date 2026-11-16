package service

import (
	"context"
	"log"

	"vuathau/backend/internal/province"
)

// assignProvince: gán tỉnh mới (34 tỉnh từ 1/7/2025) cho hồ sơ theo toạ độ, không có thì theo địa chỉ (kể cả tên tỉnh cũ).
func (s *houseService) assignProvince(ctx context.Context, id int64) {
	h, err := s.houses.Get(ctx, id)
	if err != nil {
		return
	}
	code := ""
	if p := province.Find(h.Lat, h.Lng, h.Address); p != nil {
		code = p.Code
	}
	if code == h.Province {
		return
	}
	if err := s.houses.SetProvince(ctx, id, code); err != nil {
		log.Printf("gán tỉnh hồ sơ #%d: %v", id, err)
	}
}

// BackfillProvinces: gán tỉnh cho các hồ sơ cũ chưa có (chạy lúc khởi động).
func (s *houseService) BackfillProvinces(ctx context.Context) (int, error) {
	n, after := 0, int64(0)
	for {
		rows, err := s.houses.NoProvince(ctx, after, 500)
		if err != nil {
			return n, err
		}
		for _, g := range rows {
			after = g.ID
			if p := province.Find(g.Lat, g.Lng, g.Address); p != nil {
				if err := s.houses.SetProvince(ctx, g.ID, p.Code); err != nil {
					return n, err
				}
				n++
			}
		}
		if len(rows) < 500 {
			return n, nil
		}
	}
}

// ProvinceSites: 1 tỉnh trên bản đồ và số công trình đã chụp ở đó.
type ProvinceSites struct {
	Code   string   `json:"code"`
	Name   string   `json:"name"`
	Merged []string `json:"merged"`
	Total  int      `json:"total"`
	Recent int      `json:"recent"`
}
