package tests

import (
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

// Lớp Cộng tác viên trên bản đồ admin (layout 21/11/2026): mọi CTV (kể cả chưa chụp), loại theo chức năng,
// tích cực = có hồ sơ gần đây, vị trí theo công trình gần nhất. Sale / người ngoài không thấy lớp này.
func TestMapCTVPins(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	busy := e.register(t, "CTV Chăm Chỉ")
	idle := e.register(t, "CTV Mới Vào")
	e.newHouseP(t, busy, 5, 5100, 10.80, "0901 111 222")
	if _, err := e.db.Exec(t.Context(), `UPDATE users SET kind = 'CTV kinh doanh' WHERE id = $1`, idle.User.ID); err != nil {
		t.Fatal(err)
	}

	r := e.do(t, http.MethodGet, "/api/v1/map", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var m service.MapData
	decodeData(t, r, &m)
	pins := map[int64]service.CTVPin{}
	for _, p := range m.CTVPins {
		pins[p.ID] = p
	}
	b, ok := pins[busy.User.ID]
	if !ok || b.Type != "update" || b.Status != "active" || b.Count != 1 || b.Lat == nil || b.Approx || b.Phone == "" {
		t.Fatalf("CTV đã chụp: %+v", b)
	}
	i, ok := pins[idle.User.ID]
	if !ok || i.Type != "sales" || i.Status != "registered" || i.Count != 0 {
		t.Fatalf("CTV mới: %+v", i)
	}

	// sale và người ngoài không thấy lớp CTV
	sale := e.staff(t, model.RoleSale)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/map", sale.AccessToken, nil), http.StatusForbidden)
}
