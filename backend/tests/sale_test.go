package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/service"
)

// Trang sale: chỉ thấy công trình (đã gửi, không bị từ chối) + nhà thầu + ảnh. Không thấy CTV, không thấy tiền.
func TestSale(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "Trần Bí Mật")
	admin := e.staff(t, model.RoleAdmin)
	sale := e.staff(t, model.RoleSale)

	h1 := e.newHouseP(t, ctv, 5, 4100, 10.80, "0901 234 567")
	h2 := e.newHouseP(t, ctv, 5, 4200, 12.30, "0901234567") // cùng nhà thầu, SĐT viết khác
	// nháp: sale không thấy
	files := [][]byte{photo(t, 4300)}
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(1, 14.0)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	draft := *up.HouseID

	noSecret := func(what string, r apiResp) {
		t.Helper()
		s := string(r.Raw)
		for _, bad := range []string{"Trần Bí Mật", ctv.User.Username, "ctv", "amount", "bank", "status"} {
			if strings.Contains(s, bad) {
				t.Fatalf("%s lộ %q: %s", what, bad, s)
			}
		}
	}

	// Bản đồ: có tên CTV để lọc (từ 17/11/2026), vẫn không có SĐT, tiền, ngân hàng
	r = e.do(t, http.MethodGet, "/api/v1/sale/map", sale.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	for _, bad := range []string{ctv.User.Username, "amount", "bank", "status", "phone"} {
		if strings.Contains(string(r.Raw), bad) {
			t.Fatalf("map lộ %q: %s", bad, r.Raw)
		}
	}
	var m service.SaleMapData
	decodeData(t, r, &m)
	if len(m.Points) != 2 || len(m.Provinces) != 34 || len(m.CTVs) != 1 || m.CTVs[0].Name != "Trần Bí Mật" || m.CTVs[0].Total != 2 ||
		m.Points[0].CTVName != "Trần Bí Mật" {
		t.Fatalf("sale map: %s", r.Raw)
	}
	for _, p := range m.Points {
		if p.CoverID == nil {
			t.Fatalf("thiếu hình đại diện: %+v", p)
		}
		if ph := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", *p.CoverID), sale.AccessToken); ph.Status != 200 {
			t.Fatalf("sale không xem được hình đại diện: %d", ph.Status)
		}
		// cỡ thẻ (lưới thẻ trên web): tạo lần đầu, lần sau đọc file đã lưu
		for range 2 {
			if ph := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d?size=card", *p.CoverID), sale.AccessToken); ph.Status != 200 ||
				ph.Header.Get("Content-Type") != "image/jpeg" || len(ph.Raw) == 0 {
				t.Fatalf("ảnh cỡ thẻ: %d %v", ph.Status, ph.Header)
			}
		}
	}
	other := e.register(t, "CTV Khác")
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/sale/map?ctv=%d", other.User.ID), sale.AccessToken, nil), &m)
	if len(m.Points) != 0 {
		t.Fatalf("lọc CTV: %d điểm", len(m.Points))
	}
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/sale/map?ctv=%d", m.CTVs[0].ID), sale.AccessToken, nil), &m)
	if len(m.Points) != 2 {
		t.Fatalf("lọc CTV có công trình: %d điểm", len(m.Points))
	}

	// Danh sách dự án + tìm kiếm (không tìm theo tên CTV)
	r = e.do(t, http.MethodGet, "/api/v1/sale/projects", sale.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	noSecret("projects", r)
	var list service.SaleProjectList
	decodeData(t, r, &list)
	if list.Found != 2 || list.Rows[0].ID != h2 || list.Rows[0].Photos != 5 || list.Rows[0].CoverID == nil || list.Rows[0].ContractorPhone != "0901234567" {
		t.Fatalf("sale projects: %s", r.Raw)
	}
	for q, want := range map[string]int{"Lê Lợi": 2, "ABC": 2, "Bí Mật": 0, fmt.Sprintf("HS-%05d", h1): 1} {
		r = e.do(t, http.MethodGet, "/api/v1/sale/projects?q="+strings.ReplaceAll(q, " ", "+"), sale.AccessToken, nil)
		decodeData(t, r, &list)
		if list.Found != want {
			t.Fatalf("q=%q: %d, muốn %d", q, list.Found, want)
		}
	}

	// Chi tiết + ảnh
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/sale/projects/%d", h1), sale.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	noSecret("project", r)
	var d service.SaleProjectDetail
	decodeData(t, r, &d)
	if d.ID != h1 || len(d.PhotoList) != 5 || d.ContractorName != "Xây dựng ABC" || d.Lat == nil {
		t.Fatalf("sale project: %s", r.Raw)
	}
	if ph := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", d.PhotoList[0].ID), sale.AccessToken); ph.Status != 200 {
		t.Fatalf("sale photo: %d", ph.Status)
	}
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/sale/projects/%d", draft), sale.AccessToken, nil), http.StatusNotFound)
	var dr service.HouseDetail
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", draft), ctv.AccessToken, nil), &dr)
	if ph := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", dr.Photos[0].ID), sale.AccessToken); ph.Status != 404 {
		t.Fatalf("ảnh hồ sơ nháp: %d", ph.Status)
	}

	// Nhà thầu: 2 công trình gộp làm 1 (cùng SĐT), lọc dự án theo nhà thầu
	r = e.do(t, http.MethodGet, "/api/v1/sale/contractors", sale.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	noSecret("contractors", r)
	var cs struct{ Rows []repository.Contractor }
	decodeData(t, r, &cs)
	if len(cs.Rows) != 1 || cs.Rows[0].Total != 2 || cs.Rows[0].Key != "0901234567" {
		t.Fatalf("contractors: %s", r.Raw)
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/sale/projects?contractor="+cs.Rows[0].Key, sale.AccessToken, nil), &list)
	if list.Found != 2 {
		t.Fatalf("lọc theo nhà thầu: %d", list.Found)
	}

	// Sale không vào được dữ liệu có CTV / tiền
	for _, p := range []string{
		fmt.Sprintf("/api/v1/houses/%d", h1), fmt.Sprintf("/api/v1/houses/%d/state", h1), "/api/v1/houses", "/api/v1/map",
		"/api/v1/ctvs", "/api/v1/batches", "/api/v1/batches/1", "/api/v1/reports", "/api/v1/users", "/api/v1/users/1",
		"/api/v1/admin/queue", "/api/v1/accounting", "/api/v1/ctv/home", "/api/v1/staff",
	} {
		expectStatus(t, e.do(t, http.MethodGet, p, sale.AccessToken, nil), http.StatusForbidden)
	}
	// CTV không vào trang sale; admin vào được để kiểm tra
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/sale/map", ctv.AccessToken, nil), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/sale/projects", admin.AccessToken, nil), http.StatusOK)

	// Admin thêm được tài khoản sale
	r = e.do(t, http.MethodPost, "/api/v1/staff", admin.AccessToken, map[string]string{"login": "sale_moi", "name": "Sale Mới", "role": "sale", "password": testPass})
	expectStatus(t, r, http.StatusCreated)
	if me := e.login(t, "sale_moi", testPass); me.User.Role != model.RoleSale {
		t.Fatalf("sale login: %+v", me.User)
	}
}
