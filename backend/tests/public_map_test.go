package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/service"
)

// Bản đồ công khai (góp ý 20/11/2026): người chưa đăng nhập xem được bản đồ như sale, nhưng không tên CTV,
// không SĐT, tiền, trạng thái; ảnh đại diện chỉ mở được qua link có chữ ký, đã che SĐT.
func TestPublicMap(t *testing.T) {
	t.Parallel()
	fo := &fakeOCR{}
	e := newTestEnv(t, func(s *envSettings) { s.ocr = fo })
	ctv := e.register(t, "Lê Giấu Tên")
	e.newHouseP(t, ctv, 5, 4700, 10.80, "0901 234 567")
	e.newHouseP(t, ctv, 5, 4800, 12.30, "0907654321")

	r := e.do(t, http.MethodGet, "/api/v1/public/map", "", nil)
	expectStatus(t, r, http.StatusOK)
	for _, bad := range []string{"Lê Giấu Tên", ctv.User.Username, "0901", "0907", "amount", "bank", "status", "cover_id\":1", "phone"} {
		if strings.Contains(string(r.Raw), bad) {
			t.Fatalf("bản đồ công khai lộ %q: %s", bad, r.Raw)
		}
	}
	var m service.SaleMapData
	decodeData(t, r, &m)
	if len(m.Points) != 2 || len(m.CTVs) != 0 || len(m.Provinces) != 34 {
		t.Fatalf("bản đồ công khai: %s", r.Raw)
	}
	for _, p := range m.Points {
		if p.CTVName != "" || p.CoverID != nil || !strings.HasPrefix(p.CoverURL, "/api/v1/public/photos/") {
			t.Fatalf("điểm công khai: %+v", p)
		}
		ph := e.getRaw(t, p.CoverURL, "")
		if ph.Status != http.StatusOK || ph.Header.Get("Content-Type") != "image/jpeg" || len(ph.Raw) == 0 {
			t.Fatalf("ảnh đại diện công khai: %d", ph.Status)
		}
		// không chữ ký / chữ ký sai / đổi số ảnh: không mở được
		base, _, _ := strings.Cut(p.CoverURL, "?")
		for _, u := range []string{base, base + "?s=abc", strings.Replace(p.CoverURL, base, base+"1", 1)} {
			if g := e.getRaw(t, u, ""); g.Status == http.StatusOK {
				t.Fatalf("mở được ảnh không đúng chữ ký: %s", u)
			}
		}
	}
	if fo.calls.Load() == 0 {
		t.Fatal("ảnh công khai không qua che SĐT")
	}

	// tìm địa điểm: không cần đăng nhập, không trả công trình trong hệ thống
	r = e.do(t, http.MethodGet, "/api/v1/public/geo/search?q=xyz", "", nil)
	expectStatus(t, r, http.StatusOK)
	if strings.Contains(string(r.Raw), "Lê Giấu Tên") || !strings.Contains(string(r.Raw), `"sites":[]`) {
		t.Fatalf("tìm công khai: %s", r.Raw)
	}

	// API khác vẫn cần đăng nhập
	for _, p := range []string{"/api/v1/sale/map", "/api/v1/map", "/api/v1/houses", fmt.Sprintf("/api/v1/photos/%d", 1)} {
		if g := e.do(t, http.MethodGet, p, "", nil); g.Status != http.StatusUnauthorized {
			t.Fatalf("%s không đăng nhập: %d", p, g.Status)
		}
	}
}
