package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// Nhà thầu tự đăng ký trong app (màn Tạo tài khoản, chọn "Nhà thầu", góp ý slide 24/11/2026): vào danh sách nhà thầu
// (bảng contractors, mã dòng "app-<SĐT>") ở trạng thái chờ xác minh, giống người điền form Lark. Admin bấm
// "Xác minh & cấp tài khoản" trên trang Nhà thầu -> tài khoản app tự có (EnsureFromLark). Chưa xác minh: đăng nhập
// "Nhà thầu" được báo đang xác minh (Pending).

const appRecordPrefix = "app-"

// WithUsers: kiểm SĐT đã có tài khoản chưa khi nhà thầu tự đăng ký.
func (s *ContractorService) WithUsers(users repository.AuthRepository) *ContractorService {
	s.users = users
	return s
}

type NTRegisterInput struct {
	Name       string   `json:"name"` // người liên hệ
	Phone      string   `json:"phone"`
	Company    string   `json:"company"`
	Categories []string `json:"categories"`
	Province   string   `json:"province"`
	Ward       string   `json:"ward"`
	Street     string   `json:"street"`
}

// RegisterFromApp: lưu đăng ký nhà thầu từ app, chờ xác minh. Trả câu báo cho app.
func (s *ContractorService) RegisterFromApp(ctx context.Context, in NTRegisterInput) (string, error) {
	phone := normPhone(in.Phone)
	name, company := truncate(in.Name, 100), truncate(in.Company, 200)
	if !ValidPhone(phone) || name == "" || company == "" {
		return "", fail(ErrInvalidInput, "Cần họ tên, số điện thoại đúng (10 số) và tên công ty.")
	}
	if s.users != nil {
		if _, err := s.users.FindByLogin(ctx, phone); err == nil {
			return "", fail(ErrConflict, "Số %s đã có tài khoản. Quay lại, nhập số này để đăng nhập.", phone)
		} else if !errors.Is(err, repository.ErrNotFound) {
			return "", err
		}
	}
	regs, err := s.repo.List(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range regs {
		if ntKey(r.Phone) == ntKey(phone) && r.Verified {
			return "", fail(ErrConflict, "Số %s đã được xác minh. Chờ ít phút rồi đăng nhập bằng số này.", phone)
		}
	}
	prov, ward := strings.TrimSpace(in.Province), strings.TrimSpace(in.Ward)
	if p, w, ok := province.Ward(prov, ward); ok {
		prov, ward = p, w
	}
	cats := []string{}
	for _, c := range in.Categories {
		if c = truncate(c, 40); c != "" && len(cats) < 10 {
			cats = append(cats, c)
		}
	}
	c := &repository.RegContractor{
		LarkRecord: appRecordPrefix + phone, Name: company, Phone: phone, Contact: name, Kind: "Công ty", Role: "Nhà thầu",
		Areas: prov, Fields: cats, Verified: false, JoinedAt: time.Now(),
		Note: truncate("Đăng ký trong app. Địa chỉ: "+province.Join(truncate(in.Street, 200), ward, prov), 500),
	}
	if err := s.repo.UpsertLark(ctx, c); err != nil {
		return "", err
	}
	return fmt.Sprintf("Đã gửi đăng ký nhà thầu %s. Vua Thầu xác minh xong, nhập số %s để vào app.", company, phone), nil
}

// VerifyApp: admin xác minh nhà thầu tự đăng ký trong app (key = SĐT) và cấp tài khoản. Dòng từ form Lark thì
// xác minh trên Lark (tích "Xác minh tham gia"), không làm ở đây.
func (s *ContractorService) VerifyApp(ctx context.Context, key string) (string, error) {
	regs, err := s.repo.List(ctx)
	if err != nil {
		return "", err
	}
	for i := range regs {
		c := &regs[i]
		if ntKey(c.Phone) != key {
			continue
		}
		if !strings.HasPrefix(c.LarkRecord, appRecordPrefix) {
			return "", fail(ErrInvalidInput, "Nhà thầu này đăng ký qua form Lark: tích \"Xác minh tham gia\" trên Lark, 15 phút sau tài khoản tự có.")
		}
		if err := s.repo.SetVerified(ctx, c.ID); err != nil {
			return "", err
		}
		c.Verified = true
		if s.accounts != nil {
			if _, err := s.accounts(ctx, c); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("Đã xác minh %s. Nhà thầu nhập số %s để vào app.", c.Name, c.Phone), nil
	}
	return "", fail(ErrNotFound, "Không tìm thấy đăng ký nhà thầu.")
}
