package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// Khách hàng của CTV kinh doanh (layout bản đồ 21/11/2026, chọn: CTV kinh doanh thêm trong app + admin quản lý trên web).
//   - CTV kinh doanh (chức năng có chữ "kinh doanh"): xem / thêm / sửa / xoá khách của mình.
//   - Admin: mọi khách, thêm, sửa, đổi người phụ trách, xoá. Kế toán: xem (bản đồ).
//   - Vị trí: app gửi toạ độ (ghim tại chỗ) hoặc máy chủ tìm theo địa chỉ khi lưu.

var CustomerKinds = map[string]string{"nha_pho": "Nhà phố", "biet_thu": "Biệt thự", "can_ho": "Căn hộ"}

type CustomerInput struct {
	CTVID   *int64   `json:"ctv_id"` // admin chọn người phụ trách; CTV: bỏ qua (luôn là mình)
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Phone   string   `json:"phone"`
	Address string   `json:"address"`
	Lat     *float64 `json:"lat"`
	Lng     *float64 `json:"lng"`
	Note    string   `json:"note"`
}

// SalesCTV: CTV có làm kinh doanh (được quản lý khách hàng trong app).
func (s *houseService) SalesCTV(ctx context.Context, u *model.User) bool {
	if u.Role != model.RoleCTV || s.customers == nil {
		return false
	}
	k, err := s.customers.CTVKind(ctx, u.ID)
	return err == nil && ctvType(k) == "sales"
}

func (s *houseService) Customers(ctx context.Context, u *model.User) ([]repository.Customer, error) {
	if s.customers == nil {
		return []repository.Customer{}, nil
	}
	switch {
	case u.Role == model.RoleAdmin || u.Role == model.RoleKetoan:
		return s.customers.List(ctx, 0)
	case s.SalesCTV(ctx, u):
		return s.customers.List(ctx, u.ID)
	}
	return nil, ErrForbidden
}

// canEdit: admin mọi khách; CTV kinh doanh chỉ khách của mình.
func (s *houseService) customerFor(ctx context.Context, u *model.User, id int64) (*repository.Customer, error) {
	if s.customers == nil {
		return nil, fail(ErrNotFound, "Không tìm thấy khách hàng.")
	}
	c, err := s.customers.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrNotFound, "Không tìm thấy khách hàng.")
	}
	if err != nil {
		return nil, err
	}
	if u.Role == model.RoleAdmin {
		return c, nil
	}
	if u.Role == model.RoleCTV && c.CTVID != nil && *c.CTVID == u.ID && s.SalesCTV(ctx, u) {
		return c, nil
	}
	return nil, fail(ErrNotFound, "Không tìm thấy khách hàng.")
}

func (s *houseService) cleanCustomer(ctx context.Context, u *model.User, in CustomerInput, c *repository.Customer) error {
	c.Name = truncate(in.Name, 120)
	c.Phone = truncate(normPhone(in.Phone), 20)
	c.Address = truncate(in.Address, 300)
	c.Note = truncate(in.Note, 1000)
	c.Kind = in.Kind
	if _, ok := CustomerKinds[c.Kind]; !ok {
		c.Kind = "nha_pho"
	}
	if c.Name == "" {
		return fail(ErrInvalidInput, "Chưa lưu: cần tên khách hàng.")
	}
	if c.Phone != "" && !ValidPhone(c.Phone) {
		return fail(ErrInvalidInput, "Chưa lưu: SĐT khách hàng chưa đúng (10-11 số).")
	}
	if u.Role == model.RoleAdmin {
		c.CTVID = nil
		if in.CTVID != nil && *in.CTVID > 0 {
			k, err := s.customers.CTVKind(ctx, *in.CTVID)
			if err != nil {
				return err
			}
			if k == "" {
				return fail(ErrInvalidInput, "Chưa lưu: người phụ trách phải là cộng tác viên đang hoạt động.")
			}
			c.CTVID = in.CTVID
		}
	} else {
		c.CTVID = ptr(u.ID)
	}
	// vị trí: toạ độ gửi kèm (hợp lệ, trong VN) > tìm theo địa chỉ (đổi địa chỉ thì tìm lại)
	if in.Lat != nil && in.Lng != nil && *in.Lat > 7 && *in.Lat < 24 && *in.Lng > 101 && *in.Lng < 118 {
		c.Lat, c.Lng = in.Lat, in.Lng
	} else if c.Address != "" && s.geo != nil {
		gctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		ps, err := s.geo.Search(gctx, c.Address)
		cancel()
		if err == nil && len(ps) > 0 {
			c.Lat, c.Lng = ptr(ps[0].Lat), ptr(ps[0].Lng)
		}
	}
	return nil
}

func (s *houseService) CreateCustomer(ctx context.Context, u *model.User, in CustomerInput) (*repository.Customer, string, error) {
	if s.customers == nil || !(u.Role == model.RoleAdmin || s.SalesCTV(ctx, u)) {
		return nil, "", ErrForbidden
	}
	c := &repository.Customer{}
	if err := s.cleanCustomer(ctx, u, in, c); err != nil {
		return nil, "", err
	}
	if err := s.customers.Create(ctx, c, u.ID); err != nil {
		return nil, "", err
	}
	_ = s.logs.Add(ctx, u.ID, ActCustomer, nil, nil, "Thêm "+c.Name)
	got, err := s.customers.Get(ctx, c.ID)
	return got, s.customerMsg("Đã thêm", got), err
}

func (s *houseService) UpdateCustomer(ctx context.Context, u *model.User, id int64, in CustomerInput) (*repository.Customer, string, error) {
	c, err := s.customerFor(ctx, u, id)
	if err != nil {
		return nil, "", err
	}
	oldAddr, oldLat, oldLng := c.Address, c.Lat, c.Lng
	keepPos := in.Lat == nil && strings.TrimSpace(in.Address) == oldAddr // địa chỉ không đổi, không gửi toạ độ: giữ vị trí cũ
	if keepPos {
		in.Lat, in.Lng = oldLat, oldLng
	}
	if err := s.cleanCustomer(ctx, u, in, c); err != nil {
		return nil, "", err
	}
	if err := s.customers.Update(ctx, c); err != nil {
		return nil, "", err
	}
	_ = s.logs.Add(ctx, u.ID, ActCustomer, nil, nil, "Sửa "+c.Name)
	got, err := s.customers.Get(ctx, id)
	return got, s.customerMsg("Đã lưu", got), err
}

func (s *houseService) DeleteCustomer(ctx context.Context, u *model.User, id int64) (string, error) {
	c, err := s.customerFor(ctx, u, id)
	if err != nil {
		return "", err
	}
	if err := s.customers.Delete(ctx, id); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, u.ID, ActCustomer, nil, nil, "Xoá "+c.Name)
	return fmt.Sprintf("Đã xoá khách hàng %s.", c.Name), nil
}

func (s *houseService) customerMsg(verb string, c *repository.Customer) string {
	if c == nil {
		return verb + " khách hàng."
	}
	if c.Lat == nil {
		return fmt.Sprintf("%s khách hàng %s. Chưa tìm được vị trí theo địa chỉ: ghim vị trí hoặc ghi rõ địa chỉ hơn để hiện trên bản đồ.", verb, c.Name)
	}
	return fmt.Sprintf("%s khách hàng %s.", verb, c.Name)
}
