package tests

import (
	"fmt"
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/service"
)

// Khách hàng của CTV kinh doanh (layout 21/11/2026): CTV kinh doanh thêm trong app, chỉ thấy khách của mình;
// CTV cập nhật không có mục này; admin quản lý mọi khách, đổi người phụ trách; kế toán chỉ xem; bản đồ admin có lớp Khách hàng.
func TestCustomers(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)
	sales := e.register(t, "CTV Bán Hàng")
	sales2 := e.register(t, "CTV Bán Hàng 2")
	plain := e.register(t, "CTV Cập Nhật")
	for _, id := range []int64{sales.User.ID, sales2.User.ID} {
		if _, err := e.db.Exec(t.Context(), `UPDATE users SET kind = 'CTV kinh doanh' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	var me struct {
		Sales bool `json:"sales"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/me", sales.AccessToken, nil), &me)
	if !me.Sales {
		t.Fatal("CTV kinh doanh: /me thiếu sales")
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/me", plain.AccessToken, nil), &me)
	if me.Sales {
		t.Fatal("CTV cập nhật mà /me có sales")
	}

	type res struct {
		Customer repository.Customer `json:"customer"`
	}
	lat, lng := 10.79, 106.70
	// CTV kinh doanh thêm khách (ghim vị trí tại chỗ); gửi ctv_id khác cũng không đổi được người phụ trách
	r := e.do(t, http.MethodPost, "/api/v1/customers", sales.AccessToken, map[string]any{
		"name": "Anh Hoàng Linh", "kind": "biet_thu", "phone": "+84 979 260 259", "address": "63 Hẻm 69 Đường D2, Bình Chánh",
		"lat": lat, "lng": lng, "ctv_id": sales2.User.ID,
	})
	expectStatus(t, r, http.StatusOK)
	var c res
	decodeData(t, r, &c)
	if c.Customer.ID == 0 || c.Customer.CTVID == nil || *c.Customer.CTVID != sales.User.ID || c.Customer.Kind != "biet_thu" ||
		c.Customer.Phone != "0979260259" || c.Customer.Lat == nil || c.Customer.CTVName != "CTV Bán Hàng" {
		t.Fatalf("thêm khách: %s", r.Raw)
	}
	id := c.Customer.ID
	// thiếu tên / SĐT sai
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/customers", sales.AccessToken, map[string]any{"name": " "}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/customers", sales.AccessToken, map[string]any{"name": "A", "phone": "123"}), http.StatusBadRequest)
	// CTV cập nhật: không có quyền
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/customers", plain.AccessToken, map[string]any{"name": "X"}), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/customers", plain.AccessToken, nil), http.StatusForbidden)

	list := func(tok string) []repository.Customer {
		t.Helper()
		var d struct {
			Rows []repository.Customer `json:"rows"`
		}
		r := e.do(t, http.MethodGet, "/api/v1/customers", tok, nil)
		expectStatus(t, r, http.StatusOK)
		decodeData(t, r, &d)
		return d.Rows
	}
	if len(list(sales.AccessToken)) != 1 || len(list(sales2.AccessToken)) != 0 || len(list(admin.AccessToken)) != 1 || len(list(kt.AccessToken)) != 1 {
		t.Fatal("danh sách khách theo vai trò sai")
	}
	// CTV khác không sửa / xoá được khách không phải của mình
	path := fmt.Sprintf("/api/v1/customers/%d", id)
	expectStatus(t, e.do(t, http.MethodPut, path, sales2.AccessToken, map[string]any{"name": "Đổi"}), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodDelete, path, sales2.AccessToken, nil), http.StatusNotFound)
	// kế toán chỉ xem
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/customers", kt.AccessToken, map[string]any{"name": "X"}), http.StatusForbidden)

	// CTV sửa: địa chỉ không đổi, không gửi toạ độ: giữ vị trí cũ
	r = e.do(t, http.MethodPut, path, sales.AccessToken, map[string]any{"name": "Anh Hoàng Linh", "kind": "nha_pho", "address": "63 Hẻm 69 Đường D2, Bình Chánh"})
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &c)
	if c.Customer.Kind != "nha_pho" || c.Customer.Lat == nil || *c.Customer.Lat != lat {
		t.Fatalf("sửa khách: %s", r.Raw)
	}
	// admin đổi người phụ trách
	r = e.do(t, http.MethodPut, path, admin.AccessToken, map[string]any{"name": "Anh Hoàng Linh", "kind": "can_ho", "ctv_id": sales2.User.ID,
		"address": "63 Hẻm 69 Đường D2, Bình Chánh"})
	expectStatus(t, r, http.StatusOK)
	if len(list(sales.AccessToken)) != 0 || len(list(sales2.AccessToken)) != 1 {
		t.Fatal("đổi người phụ trách chưa ăn")
	}

	// bản đồ admin có lớp khách hàng
	var m service.MapData
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/map", admin.AccessToken, nil), &m)
	if len(m.Customers) != 1 || m.Customers[0].CTVName != "CTV Bán Hàng 2" {
		t.Fatalf("bản đồ: khách hàng %+v", m.Customers)
	}
	// bản đồ công khai không có khách hàng
	pub := e.do(t, http.MethodGet, "/api/v1/public/map", "", nil)
	expectStatus(t, pub, http.StatusOK)
	if containsAny(string(pub.Raw), "Hoàng Linh", "0979260259") {
		t.Fatal("bản đồ công khai lộ khách hàng")
	}

	// xoá
	expectStatus(t, e.do(t, http.MethodDelete, path, sales2.AccessToken, nil), http.StatusOK)
	if len(list(admin.AccessToken)) != 0 {
		t.Fatal("xoá khách chưa ăn")
	}
}

func containsAny(s string, xs ...string) bool {
	for _, x := range xs {
		if len(x) > 0 && len(s) >= len(x) {
			for i := 0; i+len(x) <= len(s); i++ {
				if s[i:i+len(x)] == x {
					return true
				}
			}
		}
	}
	return false
}
