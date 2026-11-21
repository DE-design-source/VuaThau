package service

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// Lớp Cộng tác viên trên bản đồ admin / kế toán (layout 21/11/2026): mỗi CTV 1 ghim tròn.
//   - Loại: "sales" (CTV kinh doanh: chức năng có chữ "kinh doanh" / "sale"), còn lại "update" (CTV cập nhật công trình).
//   - Trạng thái: "active" (tích cực: từ 10 công trình trong 30 ngày), "registered" (đã đăng ký).
//   - Vị trí: địa chỉ CTV bên Messenger (tìm toạ độ) > công trình gần nhất CTV chụp > khu vực / tỉnh của CTV (gần đúng).
//     Tìm toạ độ theo chữ chạy nền, nhớ lại; lần đầu CTV chưa có toạ độ thì chỉ hiện trong danh sách.

type CTVPin struct {
	ID      int64      `json:"id"`
	Name    string     `json:"name"`
	Phone   string     `json:"phone"`
	Kind    string     `json:"kind"`
	Type    string     `json:"type"`   // update | sales
	Status  string     `json:"status"` // active | registered
	Address string     `json:"address"`
	Lat     *float64   `json:"lat"`
	Lng     *float64   `json:"lng"`
	Approx  bool       `json:"approx"` // vị trí theo khu vực / tỉnh, không chính xác
	Count   int        `json:"count"`
	Recent  int        `json:"recent"` // hồ sơ gửi trong 30 ngày
	Last    *time.Time `json:"last"`
}

type ctvGeoCache struct {
	mu      sync.Mutex
	pos     map[string][2]float64 // chữ địa chỉ -> toạ độ
	miss    map[string]time.Time  // không tìm được: thử lại sau 1 ngày
	running bool
}

func ctvType(kind string) string {
	k := province.Key(kind)
	if strings.Contains(k, "kinh doanh") || strings.Contains(k, "sale") || strings.Contains(k, "ban hang") {
		return "sales"
	}
	return "update"
}

func (s *houseService) ctvPins(ctx context.Context, _ int) ([]CTVPin, error) {
	rows, err := s.houses.CTVPins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CTVPin, 0, len(rows))
	var todo []string
	s.ctvGeo.mu.Lock()
	if s.ctvGeo.pos == nil {
		s.ctvGeo.pos, s.ctvGeo.miss = map[string][2]float64{}, map[string]time.Time{}
	}
	lookup := func(q string) (float64, float64, bool) {
		q = strings.TrimSpace(q)
		if q == "" {
			return 0, 0, false
		}
		if p, ok := s.ctvGeo.pos[q]; ok {
			return p[0], p[1], true
		}
		if t, ok := s.ctvGeo.miss[q]; !ok || s.now().Sub(t) > 24*time.Hour {
			todo = append(todo, q)
		}
		return 0, 0, false
	}
	for _, r := range rows {
		p := CTVPin{ID: r.ID, Name: r.Name, Phone: r.Phone, Kind: r.Kind, Type: ctvType(r.Kind), Status: "registered", Count: r.Total, Last: r.Last, Recent: r.Recent}
		if r.Recent >= repository.ActiveHouses { // tích cực: từ 10 công trình trong 30 ngày (góp ý 21/11)
			p.Status = "active"
		}
		prov := ""
		if pv := province.ByCode(r.Region); pv != nil {
			prov = pv.Name
		}
		area := strings.TrimSpace(r.Area)
		if i := strings.IndexAny(area, "(;"); i > 0 { // "Lâm Đồng (công trình chụp ở ...)"
			area = strings.TrimSpace(area[:i])
		}
		set := func(lat, lng float64, approx bool) {
			p.Lat, p.Lng, p.Approx = ptr(lat), ptr(lng), approx
		}
		switch {
		case r.Addr != "":
			p.Address = r.Addr
			if lat, lng, ok := lookup(r.Addr); ok {
				set(lat, lng, false)
			} else if r.HouseLat != nil {
				set(*r.HouseLat, *r.HouseLng, true)
			}
		case r.HouseLat != nil:
			p.Address = r.HouseAt
			set(*r.HouseLat, *r.HouseLng, false)
		default:
			p.Address = strings.Join(nonEmpty(area, prov), ", ")
			for _, q := range nonEmpty(area, prov) {
				if lat, lng, ok := lookup(q); ok {
					set(lat, lng, true)
					break
				}
			}
		}
		out = append(out, p)
	}
	start := len(todo) > 0 && !s.ctvGeo.running && s.geo != nil
	if start {
		s.ctvGeo.running = true
	}
	s.ctvGeo.mu.Unlock()
	if start {
		go s.geocodeCTVs(todo)
	}
	return out, nil
}

// geocodeCTVs: tìm toạ độ các địa chỉ CTV (chạy nền, lần lượt: dịch vụ bản đồ giới hạn số lượt).
func (s *houseService) geocodeCTVs(qs []string) {
	defer func() {
		s.ctvGeo.mu.Lock()
		s.ctvGeo.running = false
		s.ctvGeo.mu.Unlock()
	}()
	seen := map[string]bool{}
	for _, q := range qs {
		if seen[q] {
			continue
		}
		seen[q] = true
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		ps, err := s.geo.Search(ctx, q)
		cancel()
		s.ctvGeo.mu.Lock()
		if err != nil || len(ps) == 0 {
			if err != nil {
				log.Printf("toạ độ CTV %q: %v", q, err)
			}
			s.ctvGeo.miss[q] = s.now()
		} else {
			s.ctvGeo.pos[q] = [2]float64{ps[0].Lat, ps[0].Lng}
		}
		s.ctvGeo.mu.Unlock()
	}
}

func nonEmpty(xs ...string) []string {
	out := []string{}
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
