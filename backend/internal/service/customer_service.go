package service

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/integration/dezontech"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// Khách hàng (layout bản đồ 21/11/2026, chọn: chỉ lấy từ pipeline Kinh doanh của Dezon Pro bằng khoá chỉ đọc).
// Mỗi 15 phút: đọc toàn bộ khách bên dezon.tech -> lưu bảng customers (thêm / sửa / đánh dấu xoá), rồi tìm toạ độ
// theo địa chỉ (chạy nền, lần lượt). Admin, kế toán xem trên bản đồ; Vua Thầu không sửa khách.

// LeadSource: nguồn khách hàng (dezon.tech; test dùng bản giả).
type LeadSource interface {
	Leads(ctx context.Context) ([]dezontech.Lead, error)
	StaffNames(ctx context.Context) map[string]string
}

func (s *houseService) Customers(ctx context.Context, u *model.User) ([]repository.Customer, error) {
	if u.Role != model.RoleAdmin && u.Role != model.RoleKetoan {
		return nil, ErrForbidden
	}
	if s.customers == nil {
		return []repository.Customer{}, nil
	}
	return s.customers.List(ctx)
}

// SyncCustomers: 1 lượt lấy khách từ dezon.tech. Trả số khách đang có.
func (s *houseService) SyncCustomers(ctx context.Context) (int, error) {
	if s.leads == nil || s.customers == nil {
		return 0, nil
	}
	leads, err := s.leads.Leads(ctx)
	if err != nil {
		return 0, err
	}
	names := s.leads.StaffNames(ctx)
	keep := make([]string, 0, len(leads))
	for _, l := range leads {
		key := l.Key()
		if key == "" {
			continue
		}
		owner := l.Who()
		if n, ok := names[owner]; ok {
			owner = n
		}
		area := strings.Join(nonEmpty(l.Geo.Kv, l.Geo.Tinh), ", ")
		name := strings.TrimSpace(l.Name)
		if name == "" {
			name = strings.TrimSpace(l.ProjName)
		}
		c := &repository.Customer{ExtID: key, Name: truncate(name, 200), Kind: truncate(l.ProjectType, 60), Scale: truncate(l.Scale, 60),
			Phone: truncate(normPhone(l.Phone), 30), Email: truncate(l.Email, 120), Address: truncate(l.Address, 300), Area: truncate(area, 120),
			Stage: truncate(l.Stage, 60), Source: truncate(l.Source, 60), Owner: truncate(owner, 120), Note: truncate(l.Notes, 1000)}
		if v, err := strconv.ParseFloat(l.ValueVnd.String(), 64); err == nil {
			c.ValueVnd = int64(v)
		}
		for _, f := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(f, l.CreatedAt); err == nil {
				c.LeadAt = &t
				break
			}
		}
		if err := s.customers.Upsert(ctx, c); err != nil {
			return 0, err
		}
		keep = append(keep, key)
	}
	// danh sách rỗng bất thường (lỗi bên kia trả []): không xoá hàng loạt
	if len(keep) > 0 {
		if n, err := s.customers.Prune(ctx, keep); err != nil {
			return 0, err
		} else if n > 0 {
			log.Printf("khách hàng dezon.tech: %d khách không còn, đã ẩn", n)
		}
	}
	s.geocodeCustomers(ctx)
	return len(keep), nil
}

// geocodeCustomers: tìm toạ độ khách mới / đổi địa chỉ (tối đa 60 khách mỗi lượt, dịch vụ bản đồ giới hạn tốc độ).
func (s *houseService) geocodeCustomers(ctx context.Context) {
	if s.geo == nil {
		return
	}
	todo, err := s.customers.NeedGeo(ctx, 200)
	if err != nil {
		log.Printf("khách hàng: %v", err)
		return
	}
	// trung tâm tỉnh của chữ địa chỉ / khu vực, lệch nhẹ theo mã khách (tối đa ~3 km) cho ghim khỏi đè nhau
	center := func(id int64, texts ...string) (*float64, *float64) {
		for _, t := range texts {
			if p := province.FromAddress(t); p != nil {
				if lat, lng, ok := province.Center(p.Code); ok {
					return ptr(lat + float64(id*7919%61-30)*0.001), ptr(lng + float64(id*104729%61-30)*0.001)
				}
			}
		}
		return nil, nil
	}
	for _, c := range todo {
		q := strings.Join(nonEmpty(c.Address, c.Area), ", ")
		var lat, lng *float64
		if strings.TrimSpace(c.Address) == "" {
			// chỉ có khu vực (đọc từ ghi chú): đặt ở trung tâm tỉnh, không tra địa chỉ (góp ý 22/11/2026)
			lat, lng = center(c.ID, c.Area)
		} else {
			gctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			ps, err := s.geo.Search(gctx, c.Address)
			cancel()
			if err == nil && len(ps) > 0 {
				lat, lng = ptr(ps[0].Lat), ptr(ps[0].Lng)
			} else {
				lat, lng = center(c.ID, c.Address, c.Area) // địa chỉ không tra được: trung tâm tỉnh
			}
		}
		if err := s.customers.SetGeo(ctx, c.ID, q, lat, lng); err != nil {
			log.Printf("khách hàng #%d: %v", c.ID, err)
		}
	}
}

// StartCustomerSync: lấy khách từ dezon.tech mỗi every (không có khoá: không chạy).
func (s *houseService) StartCustomerSync(every time.Duration) {
	if s.leads == nil || s.customers == nil {
		log.Printf("Khách hàng dezon.tech: tắt (chưa có DEZON_DB_URL / DEZON_TECH_KEY)")
		return
	}
	log.Printf("Khách hàng dezon.tech: bật, mỗi %s", every)
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			n, err := s.SyncCustomers(ctx)
			cancel()
			if err != nil {
				log.Printf("khách hàng dezon.tech: %v", err)
			} else {
				log.Printf("khách hàng dezon.tech: %d khách", n)
			}
			time.Sleep(every)
		}
	}()
}
