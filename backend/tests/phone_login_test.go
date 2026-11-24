package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/model"
)

// App vào bằng SĐT (góp ý 20/11/2026): SĐT có tài khoản CTV thì vào luôn, chưa có thì sang Đăng ký (không cần mật khẩu).
// Ai biết SĐT cũng vào được nên CTV không tự đổi ngân hàng đã lưu; admin đổi trên web.
func TestPhoneLogin(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	type phoneRes struct {
		Registered  bool       `json:"registered"`
		Phone       string     `json:"phone"`
		AccessToken string     `json:"access_token"`
		User        model.User `json:"user"`
	}
	phoneLogin := func(p string) (apiResp, phoneRes) {
		t.Helper()
		r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": p})
		var d phoneRes
		if r.Status == http.StatusOK {
			decodeData(t, r, &d)
		}
		return r, d
	}

	// SĐT chưa có: báo chưa đăng ký, không cấp token
	phone := e.newPhone()
	r, d := phoneLogin("+84 " + phone[1:])
	expectStatus(t, r, http.StatusOK)
	if d.Registered || d.AccessToken != "" || d.Phone != phone {
		t.Fatalf("SĐT mới: %s", r.Raw)
	}

	// Đăng ký không mật khẩu
	body := ctvBody("CTV vào bằng SĐT", phone)
	delete(body, "password")
	reg := e.do(t, http.MethodPost, "/api/v1/auth/register", "", body)
	expectStatus(t, reg, http.StatusCreated)
	assertNoPasswordLeak(t, reg)

	// Lần sau: chỉ SĐT là vào (viết có dấu chấm, cách vẫn nhận)
	r, d = phoneLogin(phone[:4] + "." + phone[4:7] + " " + phone[7:])
	expectStatus(t, r, http.StatusOK)
	if !d.Registered || d.AccessToken == "" || d.User.Role != model.RoleCTV {
		t.Fatalf("SĐT đã đăng ký: %s", r.Raw)
	}
	assertNoPasswordLeak(t, r)
	tok := d.AccessToken
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", tok, nil), http.StatusOK)

	// màn đăng nhập có nút chọn vai trò: CTV chọn "Nhà thầu" -> 409 kèm hướng dẫn; chọn đúng thì vào
	r = e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone, "as": "nhathau"})
	expectStatus(t, r, http.StatusConflict)
	if !strings.Contains(string(r.Raw), "Cộng tác viên") {
		t.Fatalf("chọn nhầm vai trò: %s", r.Raw)
	}
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone, "as": "ctv"}), http.StatusOK)
	// nhà thầu chọn "Cộng tác viên" -> 409
	ntAdmin := e.staff(t, model.RoleAdmin)
	ntPhone := e.newPhone()
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", ntAdmin.AccessToken, map[string]string{"phone": ntPhone, "name": "NT"}), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": ntPhone, "as": "ctv"}), http.StatusConflict)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": ntPhone, "as": "nhathau"}), http.StatusOK)

	// SĐT sai
	r, _ = phoneLogin("12345")
	expectStatus(t, r, http.StatusBadRequest)

	// Tài khoản nhân viên không vào bằng SĐT
	admin := e.staff(t, model.RoleAdmin)
	staffPhone := e.newPhone()
	if _, err := e.db.Exec(t.Context(), `INSERT INTO users (username, name, role, password_hash) VALUES ($1, 'Kế toán SĐT', 'ketoan', 'x')`, staffPhone); err != nil {
		t.Fatal(err)
	}
	r, _ = phoneLogin(staffPhone)
	if r.Status == http.StatusOK || !strings.Contains(string(r.Raw), "mật khẩu") {
		t.Fatalf("nhân viên vào bằng SĐT: %d %s", r.Status, r.Raw)
	}

	// Ngân hàng đã có (lúc đăng ký): CTV không tự đổi được
	nb := map[string]string{"bank_name": "Techcombank", "bank_no": "9999999999", "bank_owner": "ke xau"}
	r = e.do(t, http.MethodPut, "/api/v1/me/bank", tok, nb)
	expectStatus(t, r, http.StatusForbidden)
	if !strings.Contains(string(r.Raw), "admin") {
		t.Fatalf("thông báo khoá ngân hàng: %s", r.Raw)
	}

	// Admin đổi giúp, có ghi nhật ký
	id := d.User.ID
	r = e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/ctvs/%d/bank", id), admin.AccessToken, nb)
	expectStatus(t, r, http.StatusOK)
	var bank string
	if err := e.db.QueryRow(t.Context(), `SELECT bank FROM users WHERE id = $1`, id).Scan(&bank); err != nil || !strings.Contains(bank, "9999999999") {
		t.Fatalf("admin đổi ngân hàng: %v %q", err, bank)
	}
	// CTV / kế toán không gọi được API đổi ngân hàng của admin
	expectStatus(t, e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/ctvs/%d/bank", id), tok, nb), http.StatusForbidden)

	// CTV chưa có ngân hàng (tạo từ Messenger): lần đầu tự nhập được
	p2 := e.newPhone()
	if _, err := e.db.Exec(t.Context(), `INSERT INTO users (username, name, role, password_hash) VALUES ($1, 'CTV chưa có NH', 'ctv', 'x')`, p2); err != nil {
		t.Fatal(err)
	}
	_, d2 := phoneLogin(p2)
	if !d2.Registered {
		t.Fatal("CTV tạo sẵn không vào được bằng SĐT")
	}
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/me/bank", d2.AccessToken, nb), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/me/bank", d2.AccessToken, nb), http.StatusForbidden)
}
