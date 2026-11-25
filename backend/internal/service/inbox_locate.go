package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/province"
)

// Hồ sơ Messenger: ảnh CTV gửi qua Messenger không có GPS nên bị "Không có vị trí" (0 ảnh đạt, tiền 0) tới khi có
// địa chỉ. Admin điền / đổi địa chỉ hồ sơ (góp ý 25/11/2026) thì:
//   - ảnh không GPS lấy địa chỉ đó làm vị trí (hết lỗi "Không có vị trí"; KHÔNG ghi toạ độ giả vào ảnh);
//   - hồ sơ không ảnh nào có GPS: tra toạ độ gần đúng từ địa chỉ (ghim bản đồ, kiểm trùng), chỉ nhận điểm nằm trong
//     đúng tỉnh ghi trong địa chỉ (tra địa chỉ hay ra chỗ khác hẳn; ghim sai còn tệ hơn chưa có ghim);
//   - hồ sơ đang chờ duyệt: tính lại tiền theo số ảnh đạt; bỏ nhãn "Chờ phân loại".
//
// Trả câu thông báo thêm cho admin ("" nếu không đổi gì).
func (s *houseService) locateInbox(ctx context.Context, by *model.User, h *model.House, addr string) string {
	n, err := s.photos.FillAddress(ctx, h.ID, h.Address, addr)
	if err != nil {
		log.Printf("hồ sơ #%d: điền vị trí ảnh theo địa chỉ: %v", h.ID, err)
		return ""
	}
	ps, err := s.photos.ListByHouse(ctx, h.ID)
	if err != nil {
		log.Printf("hồ sơ #%d: %v", h.ID, err)
		return ""
	}
	gps := false
	for _, p := range ps {
		if p.Lat != nil && p.Lng != nil {
			gps = true
			break
		}
	}
	done := []string{}
	if n > 0 {
		done = append(done, fmt.Sprintf("%d ảnh không GPS lấy vị trí theo địa chỉ", n))
	}
	if !gps {
		if lat, lng := s.geocodeAddress(ctx, addr); lat != nil {
			if err := s.houses.SetLatLng(ctx, h.ID, lat, lng); err != nil {
				log.Printf("hồ sơ #%d: ghi toạ độ: %v", h.ID, err)
			} else {
				done = append(done, fmt.Sprintf("toạ độ tra từ địa chỉ (gần đúng): %.6f, %.6f", *lat, *lng))
			}
		} else if h.Lat == nil {
			done = append(done, "chưa tra được toạ độ từ địa chỉ (cần ghi rõ phường / xã, tỉnh)")
		}
	}
	if h.Status == model.StatusPending {
		st, err := s.settings.Get(ctx)
		if err == nil {
			amount, err := s.priceOf(ctx, st, h.ID, FillProblems(ps, s.now()), h.EffScope())
			if err == nil && amount != h.Amount {
				if err := s.houses.SetAmount(ctx, h.ID, amount); err == nil {
					done = append(done, fmt.Sprintf("tiền %s → %s", Money(h.Amount), Money(amount)))
				}
			}
		}
	}
	if strings.HasPrefix(h.Dup, LooseLabel) {
		rest := ""
		if _, after, ok := strings.Cut(h.Dup, ". "); ok {
			rest = after // cảnh báo trùng công trình (nếu có) giữ lại
		}
		if err := s.houses.SetDup(ctx, h.ID, rest); err != nil {
			log.Printf("hồ sơ #%d: bỏ nhãn chờ phân loại: %v", h.ID, err)
		}
	}
	if len(done) == 0 {
		return ""
	}
	msg := capFirst(strings.Join(done, ", "))
	s.note(ctx, by, ActEdit, h.ID, truncate("Theo địa chỉ: "+msg, MaxReasonLen))
	return " " + msg + "."
}

// geocodeAddress: toạ độ gần đúng của địa chỉ; nil nếu không tra được hoặc kết quả nằm ngoài tỉnh ghi trong địa chỉ.
func (s *houseService) geocodeAddress(ctx context.Context, addr string) (*float64, *float64) {
	want := province.FromAddress(addr)
	if s.geo == nil || want == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	places, err := s.geo.Search(ctx, addr)
	if err != nil {
		log.Printf("tra toạ độ %q: %v", addr, err)
		return nil, nil
	}
	for _, pl := range places {
		lat, lng := pl.Lat, pl.Lng
		if !validLatLng(&lat, &lng) {
			continue
		}
		if at := province.At(lat, lng); at == nil || at.Code != want.Code {
			continue
		}
		return &lat, &lng
	}
	return nil, nil
}
