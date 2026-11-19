package tests

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/province"
)

// Trang Cộng tác viên (19/11/2026): khu vực (tự tính / admin chọn), chức năng, ghi chú, số tài khoản; bộ lọc.
func TestCTVProfileAndFilters(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)
	a := e.register(t, "CTV Khu A")
	b := e.register(t, "CTV Khu B")
	ps := province.All()
	p1, p2 := ps[0].Code, ps[1].Code

	// A chụp 2 công trình ở p1, 1 ở p2 -> khu vực tự tính p1; B có tài khoản ngân hàng
	h1 := e.newHouse(t, a, 5, 9500, 10.71)
	h2 := e.newHouse(t, a, 5, 9600, 10.73)
	h3 := e.newHouse(t, a, 5, 9700, 10.75)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		if _, err := e.db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE houses SET province = $2, status = 'cho_duyet' WHERE id IN ($1, $3)`, h1, p1, h2)
	exec(`UPDATE houses SET province = $2, status = 'cho_duyet' WHERE id = $1`, h3, p2)
	exec(`UPDATE users SET bank = 'Vietcombank · 0123456789 · CTV KHU B' WHERE id = $1`, b.User.ID)
	exec(`UPDATE users SET bank = '' WHERE id = $1`, a.User.ID) // register() gán sẵn ngân hàng

	type row struct {
		ID         int64  `json:"id"`
		Region     string `json:"region"`
		RegionAuto bool   `json:"region_auto"`
		Kind       string `json:"kind"`
		Note       string `json:"note"`
		Bank       string `json:"bank"`
	}
	type facet struct {
		Value string `json:"value"`
		Count int    `json:"count"`
	}
	type list struct {
		Rows   []row `json:"rows"`
		Found  int   `json:"found"`
		Facets struct {
			Regions []facet `json:"regions"`
			Kinds   []facet `json:"kinds"`
		} `json:"facets"`
	}
	get := func(tok, query string) list {
		t.Helper()
		var l list
		r := e.do(t, http.MethodGet, "/api/v1/ctvs?"+query, tok, nil)
		expectStatus(t, r, http.StatusOK)
		decodeData(t, r, &l)
		return l
	}
	find := func(l list, id int64) *row {
		for i := range l.Rows {
			if l.Rows[i].ID == id {
				return &l.Rows[i]
			}
		}
		return nil
	}

	l := get(admin.AccessToken, "")
	ra, rb := find(l, a.User.ID), find(l, b.User.ID)
	if ra == nil || rb == nil || ra.Region != p1 || !ra.RegionAuto || ra.Kind != "CTV" || rb.Region != "" || rb.Bank == "" {
		t.Fatalf("list: %+v %+v", ra, rb)
	}
	if len(l.Facets.Kinds) == 0 || l.Facets.Kinds[0].Value != "CTV" {
		t.Fatalf("kinds facet: %+v", l.Facets.Kinds)
	}
	// lọc: khu vực, chưa rõ khu vực, tài khoản ngân hàng
	if l := get(admin.AccessToken, "region="+p1); find(l, a.User.ID) == nil || find(l, b.User.ID) != nil {
		t.Fatalf("filter region: %+v", l.Rows)
	}
	if l := get(admin.AccessToken, "region=-"); find(l, b.User.ID) == nil || find(l, a.User.ID) != nil {
		t.Fatalf("filter no region: %+v", l.Rows)
	}
	if l := get(kt.AccessToken, "bank=co"); find(l, b.User.ID) == nil || find(l, a.User.ID) != nil {
		t.Fatalf("filter bank: %+v", l.Rows)
	}
	if l := get(admin.AccessToken, "bank=chua"); find(l, a.User.ID) == nil || find(l, b.User.ID) != nil {
		t.Fatalf("filter no bank: %+v", l.Rows)
	}

	// danh sách chức năng: chỉ admin sửa; bỏ trùng, bỏ ô trống; trống hết: 400
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/ctv-kinds", kt.AccessToken, map[string]any{"kinds": []string{"CTV"}}), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/ctv-kinds", admin.AccessToken, map[string]any{"kinds": []string{" ", ""}}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/ctv-kinds", admin.AccessToken,
		map[string]any{"kinds": []string{"CTV", " Môi  giới ", "CTV"}}), http.StatusOK)
	var cfg struct {
		Kinds []string `json:"ctv_kinds"`
	}
	r := e.do(t, http.MethodGet, "/api/v1/config", "", nil)
	decodeData(t, r, &cfg)
	if fmt.Sprint(cfg.Kinds) != "[CTV Môi giới]" {
		t.Fatalf("config kinds: %v", cfg.Kinds)
	}

	// sửa thông tin CTV: chỉ admin; khu vực / chức năng sai: 400
	path := fmt.Sprintf("/api/v1/ctvs/%d/profile", b.User.ID)
	body := map[string]string{"region": p2, "kind": "Môi giới", "note": "Quen nhiều nhà thầu Thủ Đức"}
	expectStatus(t, e.do(t, http.MethodPut, path, kt.AccessToken, body), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"region": "xx", "kind": "CTV"}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"kind": "Không có"}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/ctvs/%d/profile", admin.User.ID), admin.AccessToken, body), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, body), http.StatusOK)

	rb = find(get(admin.AccessToken, "kind="+"M%C3%B4i%20gi%E1%BB%9Bi"), b.User.ID)
	if rb == nil || rb.Region != p2 || rb.RegionAuto || rb.Note != body["note"] {
		t.Fatalf("after save: %+v", rb)
	}
	// tìm theo ghi chú
	if l := get(admin.AccessToken, "q=Th%E1%BB%A7%20%C4%90%E1%BB%A9c"); find(l, b.User.ID) == nil {
		t.Fatalf("search note: %+v", l.Rows)
	}
	// trang 1 CTV có profile; ghi chú không lộ ra app CTV (/me)
	var d struct {
		Profile struct {
			Region, Kind, Note string
		} `json:"profile"`
	}
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/ctvs/%d", b.User.ID), kt.AccessToken, nil)
	decodeData(t, r, &d)
	if d.Profile.Region != p2 || d.Profile.Kind != "Môi giới" {
		t.Fatalf("detail profile: %+v", d.Profile)
	}
	r = e.do(t, http.MethodGet, "/api/v1/me", b.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	if strings.Contains(string(r.Raw), "Thủ Đức") {
		t.Fatalf("note leaked to CTV: %s", r.Raw)
	}
}
