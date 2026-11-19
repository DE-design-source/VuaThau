package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
)

// Sửa đồng bộ Messenger (góp ý 19/11/2026):
//   - hồ sơ đang chờ duyệt có thêm ảnh thì tính lại tiền.
//   - Lỗi 2: chatbot ghi 2 hồ sơ cho cùng 1 bộ ảnh, bản sau có địa chỉ chi tiết -> chép địa chỉ sang hồ sơ đã nhận.
//   - Lỗi 1: hồ sơ Messenger mặc định "chỉ chụp ngoài" (tiền tối đa price_out), admin đổi loại chụp khi duyệt.
func TestInboxFixes(t *testing.T) {
	t.Parallel()
	src := &fakeInbox{images: map[string][]byte{}}
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src })
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)
	ctx := context.Background()
	sync := func() {
		t.Helper()
		expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil), http.StatusOK)
	}
	type house struct {
		id     int64
		addr   string
		amount int64
		scope  string
		photos int
	}
	get := func(ext string) (house, bool) {
		var h house
		err := e.db.QueryRow(ctx, `SELECT h.id, h.address, h.amount, h.scope, (SELECT count(*) FROM photos p WHERE p.house_id = h.id)
			FROM houses h WHERE ext_id = $1`, ext).Scan(&h.id, &h.addr, &h.amount, &h.scope, &h.photos)
		return h, err == nil
	}

	// --- Lỗi 2: 2 hồ sơ cùng bộ ảnh ---
	son := inbox.Contact{ID: "44444444-aaaa-4bbb-8ccc-000000000004", Name: "Sơn Huỳnh", Phone: "0707578271"}
	src.add(t, "dup-a", son, 7000, 6, site("Đà Nẵng", "0912000111", "16.0544°N, 108.2022°E"))
	src.mu.Lock()
	b := src.logs[len(src.logs)-1]
	b.ID, b.UpdatedAt = "dup-b", b.UpdatedAt.Add(time.Millisecond)
	b.Fields = site("07 Hối Kiếng 08, P. Ngũ Hành Sơn, Đà Nẵng", "0912000111", "16.0544°N, 108.2022°E")
	src.logs = append(src.logs, b)
	src.mu.Unlock()

	sync()

	a, okA := get("dup-a")
	if _, okB := get("dup-b"); !okA || okB {
		t.Fatalf("dup: a=%v b=%v", okA, okB)
	}
	if a.addr != "07 Hối Kiếng 08, P. Ngũ Hành Sơn, Đà Nẵng" {
		t.Fatalf("địa chỉ chưa chép từ bản ghi trùng: %q", a.addr)
	}
	// Lỗi 1: Messenger mặc định chỉ chụp ngoài: 6 ảnh vẫn chỉ 35.000đ
	if a.scope != model.ScopeOut || a.amount != 35000 {
		t.Fatalf("scope/amount: %+v", a)
	}

	// --- hồ sơ đang chờ duyệt có thêm ảnh: tính lại tiền, ghi lịch sử ---
	// (hạ tạm về 1 ảnh để thấy tiền đổi)
	if _, err := e.db.Exec(ctx, `DELETE FROM photos WHERE house_id = $1 AND id NOT IN (SELECT min(id) FROM photos WHERE house_id = $1)`, a.id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(ctx, `UPDATE houses SET amount = 0 WHERE id = $1`, a.id); err != nil {
		t.Fatal(err)
	}
	src.mu.Lock()
	for i := range src.logs {
		if src.logs[i].ID == "dup-a" {
			src.logs[i].UpdatedAt = time.Now().Add(time.Hour)
		}
	}
	src.mu.Unlock()
	sync()
	a, _ = get("dup-a")
	if a.photos != 6 || a.amount != 35000 {
		t.Fatalf("tính lại tiền: %+v", a)
	}

	// --- đọc lại 1 lần (resyncKey): gỡ ảnh lẻ từng gom vào hồ sơ chờ duyệt (không có trong bản ghi chatbot) ---
	for i := range 3 {
		if _, err := e.db.Exec(ctx, `INSERT INTO photos (house_id, sha256, dhash, mime) VALUES ($1, $2, 'ffffffffffffffff', 'image/jpeg')`,
			a.id, fmt.Sprintf("%064d", 777+i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.db.Exec(ctx, `DELETE FROM sync_state WHERE k = 'inbox_resync_v3'`); err != nil {
		t.Fatal(err)
	}
	sync()
	if a, _ = get("dup-a"); a.photos != 6 || a.amount != 35000 {
		t.Fatalf("gỡ ảnh lẻ: %+v", a)
	}
	var pruned int
	_ = e.db.QueryRow(ctx, `SELECT count(*) FROM house_logs WHERE house_id = $1 AND note LIKE 'Messenger: gỡ 3 ảnh%'`, a.id).Scan(&pruned)
	if pruned != 1 {
		t.Fatalf("lịch sử gỡ ảnh lẻ: %d", pruned)
	}

	// --- admin đổi loại chụp: trong + ngoài = 50.000đ; kế toán không đổi được; sai loại: 400 ---
	path := fmt.Sprintf("/api/v1/houses/%d/scope", a.id)
	expectStatus(t, e.do(t, http.MethodPut, path, kt.AccessToken, map[string]string{"scope": model.ScopeBoth}), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"scope": "xx"}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"scope": model.ScopeBoth}), http.StatusOK)
	a, _ = get("dup-a")
	if a.scope != model.ScopeBoth || a.amount != 50000 {
		t.Fatalf("đổi loại chụp: %+v", a)
	}
	var d struct {
		Pay      int64            `json:"pay"`
		PayScope map[string]int64 `json:"pay_scope"`
	}
	r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", a.id), admin.AccessToken, nil)
	decodeData(t, r, &d)
	if d.Pay != 50000 || d.PayScope[model.ScopeOut] != 35000 || d.PayScope[model.ScopeBoth] != 50000 {
		t.Fatalf("detail pay: %+v", d)
	}

	// --- cấu hình tiền hồ sơ chỉ chụp ngoài ---
	r = e.do(t, http.MethodGet, "/api/v1/settings", admin.AccessToken, nil)
	var st model.Settings
	decodeData(t, r, &st)
	if st.PriceOut != 35000 {
		t.Fatalf("price_out mặc định: %+v", st)
	}
	st.PriceOut = 60000 // lớn hơn tiền cao nhất: 400
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken, st), http.StatusBadRequest)
	st.PriceOut = 30000
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken, st), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"scope": model.ScopeOut}), http.StatusOK)
	if a, _ = get("dup-a"); a.amount != 30000 {
		t.Fatalf("chỉ ngoài theo cấu hình mới: %+v", a)
	}
}
