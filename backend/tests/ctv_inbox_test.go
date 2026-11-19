package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
)

// Trang CTV (góp ý 19/11/2026): đồng bộ đủ thông tin CTV bên Messenger (SĐT, email, nhãn, ghi chú, nhu cầu, khu vực,
// số tài khoản), điền ô trống của web; CTV chưa có SĐT thì admin nhập SĐT làm tên đăng nhập. Và nút Tách hồ sơ:
// chatbot gắn ảnh nhiều công trình vào 1 hồ sơ -> admin tách ra hồ sơ mới.
func TestCTVInboxInfoAndSplit(t *testing.T) {
	t.Parallel()
	src := &fakeInbox{images: map[string][]byte{}, people: map[string]inbox.ContactInfo{}}
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src })
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)
	ctx := context.Background()
	sync := func() {
		t.Helper()
		expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil), http.StatusOK)
	}

	duy := inbox.Contact{ID: "66666666-aaaa-4bbb-8ccc-000000000006", Name: "Thanh Duy"} // chưa có SĐT
	src.add(t, "duy-1", duy, 9100, 10, site("33 Sơn Thủy 3, P. Ngũ Hành Sơn, Đà Nẵng", "0912000333", "16.0°N, 108.2°E"))
	src.people[duy.ID] = inbox.ContactInfo{Name: "Thanh Duy", Email: "duy@example.com", Tags: []string{"CTV"},
		Notes: "Chụp khu Lâm Đồng", Fields: map[string]string{"khu_vuc": "Lâm Đồng", "need": "CTV chụp ảnh"},
		Labels: map[string]string{"khu_vuc": "Khu vực", "need": "Nhu cầu"}}
	sync()

	var cid int64
	var login string
	if err := e.db.QueryRow(ctx, `SELECT id, username FROM users WHERE inbox_contact = $1`, duy.ID).Scan(&cid, &login); err != nil {
		t.Fatal(err)
	}
	type detail struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Inbox struct {
			Email  string            `json:"email"`
			Phone  string            `json:"phone"`
			Fields map[string]string `json:"fields"`
			Tags   []string          `json:"tags"`
		} `json:"inbox"`
		LoginPhone bool `json:"login_phone"`
		Profile    struct {
			Region string `json:"region"`
			Note   string `json:"note"`
		} `json:"profile"`
	}
	get := func() detail {
		t.Helper()
		var d detail
		decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/ctvs/%d", cid), admin.AccessToken, nil), &d)
		return d
	}
	d := get()
	if d.LoginPhone || d.Inbox.Email != "duy@example.com" || d.Inbox.Fields["need"] != "CTV chụp ảnh" || len(d.Inbox.Tags) != 1 {
		t.Fatalf("thông tin Messenger: %+v", d)
	}
	if d.Profile.Region == "" || d.Profile.Note != "Chụp khu Lâm Đồng" {
		t.Fatalf("điền ô trống từ Messenger: %+v", d.Profile)
	}

	// admin nhập SĐT đăng nhập; kế toán không được; sai số: 400; trùng tài khoản khác: 409
	path := fmt.Sprintf("/api/v1/ctvs/%d/login", cid)
	expectStatus(t, e.do(t, http.MethodPut, path, kt.AccessToken, map[string]string{"phone": "0905111222"}), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"phone": "12"}), http.StatusBadRequest)
	other := e.register(t, "CTV khác")
	var otherLogin string
	_ = e.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, other.User.ID).Scan(&otherLogin)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"phone": otherLogin}), http.StatusConflict)
	expectStatus(t, e.do(t, http.MethodPut, path, admin.AccessToken, map[string]string{"phone": "0905 111 222"}), http.StatusOK)
	if d = get(); !d.LoginPhone || d.User.Username != "0905111222" {
		t.Fatalf("SĐT đăng nhập: %+v", d.User)
	}

	// CTV Messenger khác chưa có SĐT, sau đó Messenger có SĐT: tự lấy làm tên đăng nhập
	son := inbox.Contact{ID: "77777777-aaaa-4bbb-8ccc-000000000007", Name: "Sơn"}
	src.add(t, "son-1", son, 9300, 6, site("Đà Nẵng", "0912000444", "16.1°N, 108.2°E"))
	sync()
	src.mu.Lock()
	src.people[son.ID] = inbox.ContactInfo{Name: "Sơn", Phone: "+84 707 578 999", Fields: map[string]string{}}
	src.mu.Unlock()
	sync()
	var sonLogin string
	_ = e.db.QueryRow(ctx, `SELECT username FROM users WHERE inbox_contact = $1`, son.ID).Scan(&sonLogin)
	if sonLogin != "0707578999" {
		t.Fatalf("SĐT Messenger chưa thành tên đăng nhập: %q", sonLogin)
	}

	// --- Tách hồ sơ: 10 ảnh, 4 ảnh là công trình khác ---
	var hid int64
	_ = e.db.QueryRow(ctx, `SELECT id FROM houses WHERE ext_id = 'duy-1'`).Scan(&hid)
	rows, err := e.db.Query(ctx, `SELECT id FROM photos WHERE house_id = $1 ORDER BY id`, hid)
	if err != nil {
		t.Fatal(err)
	}
	var pids []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		pids = append(pids, id)
	}
	rows.Close()
	sp := fmt.Sprintf("/api/v1/houses/%d/split", hid)
	body := func(ids []int64, addr string) map[string]any { return map[string]any{"photos": ids, "address": addr} }
	expectStatus(t, e.do(t, http.MethodPost, sp, kt.AccessToken, body(pids[6:], "6 Sơn Thủy 3")), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodPost, sp, admin.AccessToken, body(pids[6:], " ")), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPost, sp, admin.AccessToken, body(pids, "6 Sơn Thủy 3")), http.StatusBadRequest)
	r := e.do(t, http.MethodPost, sp, admin.AccessToken, body(append(pids[6:], 999999), "6 Sơn Thủy 3, Đà Nẵng"))
	expectStatus(t, r, http.StatusOK)
	var res struct {
		NewID int64 `json:"new_id"`
	}
	decodeData(t, r, &res)
	count := func(id int64) (n int, amount int64, status, addr string, ctv int64) {
		_ = e.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM photos WHERE house_id = h.id), amount, status, address, ctv_id
			FROM houses h WHERE id = $1`, id).Scan(&n, &amount, &status, &addr, &ctv)
		return
	}
	n1, a1, _, _, _ := count(hid)
	n2, a2, st2, addr2, ctv2 := count(res.NewID)
	if n1 != 6 || n2 != 4 || st2 != model.StatusPending || addr2 != "6 Sơn Thủy 3, Đà Nẵng" || ctv2 != cid {
		t.Fatalf("tách: cũ %d ảnh; mới %d ảnh %s %q ctv %d", n1, n2, st2, addr2, ctv2)
	}
	if a1 != 35000 || a2 != 0 { // Messenger mặc định chỉ chụp ngoài; 4 ảnh chưa đủ ảnh tối thiểu
		t.Fatalf("tiền sau tách: %d, %d", a1, a2)
	}
	var logs int
	_ = e.db.QueryRow(ctx, `SELECT count(*) FROM house_logs WHERE action = 'Tách hồ sơ' AND house_id IN ($1, $2)`, hid, res.NewID).Scan(&logs)
	if logs != 2 {
		t.Fatalf("lịch sử tách: %d", logs)
	}
	// đồng bộ lại không kéo ảnh đã tách về hồ sơ cũ
	src.mu.Lock()
	for i := range src.logs {
		if src.logs[i].ID == "duy-1" {
			src.logs[i].UpdatedAt = src.logs[i].UpdatedAt.Add(1e12)
		}
	}
	src.mu.Unlock()
	sync()
	if n1, _, _, _, _ = count(hid); n1 != 6 {
		t.Fatalf("đồng bộ lại kéo ảnh về: %d", n1)
	}
}
