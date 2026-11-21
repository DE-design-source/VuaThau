package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"vuathau/backend/internal/integration/dezontech"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

// fakeLeads: pipeline Kinh doanh giả (dạng dữ liệu như dezon.tech /api/v1/sales/leads).
type fakeLeads struct {
	mu    sync.Mutex
	leads []dezontech.Lead
}

func (f *fakeLeads) Leads(context.Context) ([]dezontech.Lead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dezontech.Lead(nil), f.leads...), nil
}
func (f *fakeLeads) StaffNames(context.Context) map[string]string { return map[string]string{"u7": "Bùi Minh Phong"} }

func lead(t *testing.T, raw string) dezontech.Lead {
	t.Helper()
	var l dezontech.Lead
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

// Khách hàng lấy từ pipeline Kinh doanh của Dezon Pro (chọn 21/11/2026): đồng bộ thêm / sửa / ẩn khách đã xoá,
// đổi mã người phụ trách ra tên, chỉ admin / kế toán xem, có trên bản đồ admin, không lộ ra bản đồ công khai.
func TestCustomersFromDezon(t *testing.T) {
	t.Parallel()
	fl := &fakeLeads{}
	e := newTestEnv(t, func(s *envSettings) { s.leads = fl })
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)
	ctv := e.register(t, "CTV xem khách")
	fl.leads = []dezontech.Lead{
		lead(t, `{"id":"L1","name":"Anh Hoàng Linh","phone":"+84 979 260 259","address":"63 Hẻm 69 Đường D2","geo":{"kv":"Bình Chánh","tinh":"Hồ Chí Minh"},
			"projectType":"Biệt thự","scale":"200 - 500 m²","valueVnd":3500000000,"stage":"tiep-can","source":"Facebook","manager":"u7","createdAt":"2026-11-20T08:00:00Z"}`),
		lead(t, `{"id":42,"name":"","projName":"Nhà phố Q7","projectType":"Nhà phố","creator":"Trần Ngọc Vy"}`),
	}
	ctx := context.Background()
	n, err := e.house.SyncCustomers(ctx)
	if err != nil || n != 2 {
		t.Fatalf("đồng bộ: %d %v", n, err)
	}
	var m service.MapData
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/map", admin.AccessToken, nil), &m)
	if len(m.Customers) != 2 {
		t.Fatalf("bản đồ: %+v", m.Customers)
	}
	by := map[string]int{}
	for i, c := range m.Customers {
		by[c.ExtID] = i
	}
	c1, c2 := m.Customers[by["L1"]], m.Customers[by["42"]]
	if c1.Owner != "Bùi Minh Phong" || c1.Phone != "0979260259" || c1.Kind != "Biệt thự" || c1.Area != "Bình Chánh, Hồ Chí Minh" || c1.ValueVnd != 3500000000 ||
		c1.LeadAt == nil {
		t.Fatalf("khách 1: %+v", c1)
	}
	if c2.Name != "Nhà phố Q7" || c2.Owner != "Trần Ngọc Vy" {
		t.Fatalf("khách 2: %+v", c2)
	}

	// đổi bên Dezon: sửa tên, xoá khách 42
	fl.mu.Lock()
	fl.leads = []dezontech.Lead{lead(t, `{"id":"L1","name":"Anh Hoàng Linh (VIP)","projectType":"Căn hộ","manager":"u7"}`)}
	fl.mu.Unlock()
	if _, err := e.house.SyncCustomers(ctx); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Rows []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"rows"`
	}
	r := e.do(t, http.MethodGet, "/api/v1/customers", kt.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &d)
	if len(d.Rows) != 1 || d.Rows[0].Name != "Anh Hoàng Linh (VIP)" || d.Rows[0].Kind != "Căn hộ" {
		t.Fatalf("sau đồng bộ lại: %s", r.Raw)
	}
	// bên kia trả rỗng bất thường: không xoá hàng loạt
	fl.mu.Lock()
	fl.leads = nil
	fl.mu.Unlock()
	if _, err := e.house.SyncCustomers(ctx); err != nil {
		t.Fatal(err)
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/customers", admin.AccessToken, nil), &d)
	if len(d.Rows) != 1 {
		t.Fatal("danh sách rỗng mà đã xoá hết khách")
	}

	// CTV, khách không xem được; bản đồ công khai không có khách
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/customers", ctv.AccessToken, nil), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/customers", "", nil), http.StatusUnauthorized)
	pub := e.do(t, http.MethodGet, "/api/v1/public/map", "", nil)
	if strings.Contains(string(pub.Raw), "Hoàng Linh") {
		t.Fatal("bản đồ công khai lộ khách hàng")
	}
}
