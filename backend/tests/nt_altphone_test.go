package tests

import (
	"fmt"
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
)

// SĐT phụ (24/11/2026): công trình CTV ghi số khác số đăng nhập của nhà thầu (VD Trịnh Bình / Hồng Phát Group).
// Admin gán số phụ -> app nhà thầu thấy thêm công trình đó và chụp nhật ký vào được; số phụ không trùng tài khoản khác.
func TestNhaThauAltPhones(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV chụp Hồng Phát")
	phone, alt, otherNT := e.newPhone(), e.newPhone(), e.newPhone()
	h1 := e.newHouseP(t, ctv, 5, 9610, 10.73, phone)
	h2 := e.newHouseP(t, ctv, 5, 9710, 10.74, alt)
	for _, p := range []string{phone, otherNT} {
		expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken, map[string]string{"phone": p, "name": "NT " + p}), http.StatusOK)
	}
	var list struct {
		Rows []struct {
			ID    int64  `json:"id"`
			Phone string `json:"phone"`
		} `json:"rows"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/admin/nt", admin.AccessToken, nil), &list)
	var uid int64
	for _, r := range list.Rows {
		if r.Phone == phone {
			uid = r.ID
		}
	}
	var lg struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone, "as": "nhathau"}), &lg)
	sites := func() map[int64]bool {
		t.Helper()
		var s struct {
			Rows []struct {
				ID int64 `json:"id"`
			} `json:"rows"`
		}
		decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/sites", lg.AccessToken, nil), &s)
		got := map[int64]bool{}
		for _, r := range s.Rows {
			got[r.ID] = true
		}
		return got
	}
	if got := sites(); len(got) != 1 || !got[h1] {
		t.Fatalf("trước khi gán số phụ: %v", got)
	}
	path := fmt.Sprintf("/api/v1/admin/nt/%d/alt-phones", uid)
	// không hợp lệ / trùng số đăng nhập tài khoản khác / không phải admin
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]any{"phones": []string{"123"}}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]any{"phones": []string{otherNT}}), http.StatusConflict)
	expectStatus(t, e.do(t, http.MethodPut, path, lg.AccessToken, map[string]any{"phones": []string{alt}}), http.StatusForbidden)
	// gán số phụ dạng +84: chuẩn hoá về 0xxx, thấy thêm công trình h2 và nhận chụp nhật ký được
	var res struct {
		AltPhones []string `json:"alt_phones"`
	}
	decodeData(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]any{"phones": []string{"+84" + alt[1:], phone}}), &res)
	if len(res.AltPhones) != 1 || res.AltPhones[0] != alt {
		t.Fatalf("số phụ lưu: %+v", res)
	}
	if got := sites(); len(got) != 2 || !got[h1] || !got[h2] {
		t.Fatalf("sau khi gán số phụ: %v", got)
	}
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/nt/sites/%d/claim", h2), lg.AccessToken, map[string]string{}), http.StatusOK)
	// nhà thầu khác không lấy được số phụ này
	var other int64
	for _, r := range list.Rows {
		if r.Phone == otherNT {
			other = r.ID
		}
	}
	expectStatus(t, e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/admin/nt/%d/alt-phones", other), admin.AccessToken, map[string]any{"phones": []string{alt}}), http.StatusConflict)
	// bỏ số phụ: còn công trình số chính (h2 đã thành dự án nên vẫn ở danh sách dự án)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]any{"phones": []string{}}), http.StatusOK)
	if got := sites(); !got[h1] || got[h2] {
		t.Fatalf("sau khi bỏ số phụ: %v", got)
	}
}
