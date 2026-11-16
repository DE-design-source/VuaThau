package tests

import (
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

// Bản đồ trong app CTV: công trình của mình đủ thông tin; của CTV khác chỉ có toạ độ.
func TestCTVMap(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "CTV A")
	b := e.register(t, "Người Khác Bí Mật")
	ha := e.newHouseP(t, a, 5, 5100, 10.80, "0901 111 222")
	e.newHouseP(t, b, 5, 5200, 12.30, "0988 777 666")

	r := e.do(t, http.MethodGet, "/api/v1/ctv/map", a.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var m service.CTVMap
	decodeData(t, r, &m)
	if len(m.Mine) != 1 || m.Mine[0].ID != ha || m.Mine[0].Status != model.StatusPending || m.Mine[0].Address == "" || !m.Mine[0].Recent || !m.Mine[0].VIP {
		t.Fatalf("mine: %s", r.Raw)
	}
	if len(m.Others) != 1 || m.Others[0].Lat != 12.30 || !m.Others[0].Recent || m.DupDays != 30 {
		t.Fatalf("others: %s", r.Raw)
	}
	// Không lộ gì của CTV B ngoài toạ độ
	for _, bad := range []string{"Người Khác Bí Mật", "0988", b.User.Username} {
		if strings.Contains(string(r.Raw), bad) {
			t.Fatalf("lộ %q: %s", bad, r.Raw)
		}
	}
	// Chỉ CTV
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/ctv/map", e.staff(t, model.RoleSale).AccessToken, nil), http.StatusForbidden)
}
