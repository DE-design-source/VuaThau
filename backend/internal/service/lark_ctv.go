package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"vuathau/backend/internal/integration/lark"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/safe"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// CTV từ Lark (góp ý 21/11/2026): bảng "Form Tuyển CTV tư vấn công trình - Vua Thầu" trên Lark Base.
// Dòng nào được tích "Duyệt tham gia" thì tạo tài khoản CTV kinh doanh (đăng nhập app bằng SĐT), trạng thái "đã đăng ký",
// khu vực theo tỉnh / thành trong form. Đã có tài khoản (cùng SĐT): chỉ điền khu vực / ghi chú còn trống.
// Bỏ tích sau đó: không xoá tài khoản (admin khoá trên web nếu cần).

// LarkCTVKind: chức năng của CTV tạo từ form tuyển CTV tư vấn công trình (góp ý 22/11/2026).
const LarkCTVKind = "CTV kinh doanh"

// LarkCTVSource: nguồn dòng form tuyển CTV (Lark; test dùng bản giả).
type LarkCTVSource interface {
	Records(ctx context.Context) ([]lark.Record, error)
}

// col: tìm giá trị theo tên cột (bỏ dấu, không phân biệt hoa thường, chỉ cần chứa cụm từ).
func col(f map[string]any, keys ...string) any {
	for name, v := range f {
		k := province.Key(name)
		for _, want := range keys {
			if strings.Contains(k, want) {
				return v
			}
		}
	}
	return nil
}

// SyncLarkCTVs: 1 lượt đọc form tuyển CTV. Trả số CTV mới tạo, số dòng đã duyệt.
func (s *userService) SyncLarkCTVs(ctx context.Context) (created, approved int, err error) {
	if s.lark == nil {
		return 0, 0, nil
	}
	recs, err := s.lark.Records(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, r := range recs {
		if !lark.Checked(col(r.Fields, "duyet tham gia")) {
			continue
		}
		phone := normPhone(lark.Text(col(r.Fields, "so dien thoai", "sdt")))
		name := truncate(lark.Text(col(r.Fields, "ho va ten", "ho ten")), 100)
		if !ValidPhone(phone) || name == "" {
			continue
		}
		approved++
		region := ""
		tinh := lark.Text(col(r.Fields, "tinh thanh", "thanh pho")) // không dò "tinh" trơn: trùng cột "Độ nhiệt tình"
		if p := province.FromAddress(tinh); p != nil {
			region = p.Code
		}
		job := lark.Text(col(r.Fields, "cong viec kinh nghiem"))
		note := strings.TrimSpace(fmt.Sprintf("Form tuyển CTV (Lark)%s", map[bool]string{true: ": " + job, false: ""}[job != ""]))
		u, err := s.auth.FindByLogin(ctx, phone)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return created, approved, err
		}
		if u == nil {
			u = &model.User{Username: phone, Name: name, Role: model.RoleCTV}
			if u.PasswordHash, err = hashPassword(randomSecret()); err != nil {
				return created, approved, err
			}
			if err := s.auth.CreateUser(ctx, u); err != nil {
				if errors.Is(err, repository.ErrDuplicate) {
					continue
				}
				return created, approved, err
			}
			created++
			// form tuyển CTV tư vấn công trình: CTV kinh doanh (ghim đỏ trên bản đồ)
			_ = s.repo.SetProfile(ctx, u.ID, region, LarkCTVKind, note)
			log.Printf("Lark: tạo CTV #%d %q (duyệt tham gia)", u.ID, name)
		} else if u.Role != model.RoleCTV {
			continue // SĐT của nhân viên: bỏ qua
		}
		_ = s.repo.FillProfileIfEmpty(ctx, u.ID, region, note)
	}
	return created, approved, nil
}

// StartLarkCTVSync: đọc form tuyển CTV mỗi every (không có app Lark: không chạy).
func (s *userService) StartLarkCTVSync(every time.Duration) {
	if s.lark == nil {
		log.Printf("CTV từ form Lark: tắt (chưa có LARK_APP_ID / LARK_APP_SECRET)")
		return
	}
	log.Printf("CTV từ form Lark: bật, mỗi %s", every)
	go func() {
		for {
			safe.Run("đồng bộ CTV Lark", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				n, ok, err := s.SyncLarkCTVs(ctx)
				cancel()
				if err != nil {
					log.Printf("CTV từ form Lark: %v", err)
				} else if n > 0 {
					log.Printf("CTV từ form Lark: %d dòng đã duyệt, tạo mới %d CTV", ok, n)
				}
			})
			time.Sleep(every)
		}
	}()
}
