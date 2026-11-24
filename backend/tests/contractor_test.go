package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/integration/lark"
	"vuathau/backend/internal/model"
)

// Trang Nhà thầu (góp ý 23/11/2026): đăng ký qua form Lark + trích từ ảnh CTV, gộp theo SĐT (84xxx = 0xxx);
// chi tiết có công trình CTV chụp; kế toán xem được, CTV không; nút Thêm nhà thầu có link form.
func TestContractors(t *testing.T) {
	t.Parallel()
	fl := &fakeLark{}
	e := newTestEnv(t, func(s *envSettings) { s.ntLark = fl })
	admin := e.staff(t, "admin")
	kt := e.staff(t, "ketoan")
	ctv := e.register(t, "CTV chụp nhà thầu")
	phone := e.newPhone()
	hid := e.newHouseP(t, ctv, 5, 9100, 10.78, phone)
	other := e.newPhone()
	e.newHouseP(t, ctv, 5, 9200, 10.79, other)

	fl.recs = []lark.Record{
		larkRec(t, `{"record_id":"n1","fields":{"Tên công ty / Cá nhân":"Công ty TNHH Hùng Phát","Số điện thoại (có Zalo)":"84`+phone[1:]+`",
			"Loại hình":"Công ty","Vai trò":"Tổng thầu","Tỉnh/thành phố nhận thi công":"TP. Hồ Chí Minh","Số năm kinh nghiệm thi công":"Trên 10 năm",
			"Giá trị công trình thường nhận":"Từ 2 đến 10 tỷ","Lĩnh vực thi công":["Xây dựng phần thô","Nội thất"],"Xác minh tham gia":true,
			"Submitted on":1794155102000,"Họ tên người liên hệ":"Anh Hùng"}}`),
		larkRec(t, `{"record_id":"n2","fields":{"Tên công ty / Cá nhân":"Tổ đội Minh","Loại hình":"Tổ đội"}}`),
		larkRec(t, `{"record_id":"n3","fields":{"Ghi chú":"dòng trống"}}`),
	}
	ctx := context.Background()
	if n, err := e.nt.SyncLark(ctx); err != nil || n != 2 {
		t.Fatalf("đồng bộ form nhà thầu: %d %v", n, err)
	}
	if n, err := e.nt.SyncLark(ctx); err != nil || n != 2 { // đọc lại: cập nhật, không thêm dòng
		t.Fatalf("đọc lại: %d %v", n, err)
	}
	var cnt int
	_ = e.db.QueryRow(ctx, `SELECT count(*) FROM contractors`).Scan(&cnt)
	if cnt != 2 {
		t.Fatalf("bảng contractors có %d dòng", cnt)
	}

	type row struct {
		Key, Name, Kind, Role, Status, Areas string
		Total                                int
		Registered, Photo                    bool
	}
	var list struct {
		Rows    []row  `json:"rows"`
		FormURL string `json:"form_url"`
	}
	r := e.do(t, http.MethodGet, "/api/v1/contractors", kt.AccessToken, nil)
	if r.Status != http.StatusOK {
		t.Fatalf("kế toán xem danh sách: %d", r.Status)
	}
	_ = json.Unmarshal(r.Body.Data, &list)
	if list.FormURL == "" {
		t.Fatal("thiếu link form Thêm nhà thầu")
	}
	byKey := map[string]row{}
	for _, x := range list.Rows {
		byKey[x.Key] = x
	}
	hp := byKey[phone]
	if !hp.Registered || !hp.Photo || hp.Total != 1 || hp.Status != "Đã xác minh" || hp.Name != "Công ty TNHH Hùng Phát" || hp.Role != "Tổng thầu" {
		t.Fatalf("nhà thầu đăng ký + ảnh CTV chưa gộp đúng: %+v", hp)
	}
	if o := byKey[other]; o.Registered || !o.Photo || o.Status != "Từ ảnh CTV" {
		t.Fatalf("nhà thầu chỉ có từ ảnh: %+v", o)
	}
	found := false
	for _, x := range list.Rows {
		if x.Name == "Tổ đội Minh" {
			found = x.Registered && x.Status == "Chưa xác minh" && x.Kind == "Tổ đội"
		}
	}
	if !found {
		t.Fatal("thiếu nhà thầu đăng ký không có SĐT")
	}

	var d struct {
		Name    string `json:"name"`
		Profile *struct {
			Contact string   `json:"contact"`
			Fields  []string `json:"fields"`
		} `json:"profile"`
		Projects []struct {
			ID int64 `json:"id"`
		} `json:"projects"`
	}
	r = e.do(t, http.MethodGet, "/api/v1/contractors/"+phone, admin.AccessToken, nil)
	_ = json.Unmarshal(r.Body.Data, &d)
	if r.Status != http.StatusOK || d.Profile == nil || d.Profile.Contact != "Anh Hùng" || len(d.Profile.Fields) != 2 || len(d.Projects) != 1 || d.Projects[0].ID != hid {
		t.Fatalf("chi tiết nhà thầu: %d %+v", r.Status, d)
	}
	if r := e.do(t, http.MethodGet, "/api/v1/contractors/0000", admin.AccessToken, nil); r.Status != http.StatusNotFound {
		t.Fatalf("nhà thầu không có: %d", r.Status)
	}
	if r := e.do(t, http.MethodGet, "/api/v1/contractors", ctv.AccessToken, nil); r.Status != http.StatusForbidden {
		t.Fatalf("CTV xem trang nhà thầu: %d", r.Status)
	}

	// bản free xem chi tiết công trình (không đăng nhập): có ảnh link ký, không SĐT nhà thầu
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/public/projects/%d", hid), "", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("chi tiết công khai: %d", r.Status)
	}
	var pp map[string]any
	_ = json.Unmarshal(r.Body.Data, &pp)
	if _, leak := pp["contractor_phone"]; leak {
		t.Fatal("chi tiết công khai lộ SĐT nhà thầu")
	}
	if ps, _ := pp["photos"].([]any); len(ps) != 5 {
		t.Fatalf("chi tiết công khai: %d ảnh", len(ps))
	}
	if r := e.do(t, http.MethodGet, "/api/v1/public/projects/999999", "", nil); r.Status != http.StatusNotFound {
		t.Fatalf("công trình không có: %d", r.Status)
	}

	// quy tắc xếp ảnh (trang Cấu hình): mặc định ảnh bảng hiệu lên đầu; sai mã: 400; kế toán không sửa được
	var rules struct {
		PhotoOrder string `json:"photo_order"`
	}
	r = e.do(t, http.MethodGet, "/api/v1/settings/ai-rules", admin.AccessToken, nil)
	_ = json.Unmarshal(r.Body.Data, &rules)
	if rules.PhotoOrder != "sign_first" {
		t.Fatalf("quy tắc xếp ảnh mặc định: %q", rules.PhotoOrder)
	}
	if r := e.do(t, http.MethodPut, "/api/v1/settings/photo-order", admin.AccessToken, map[string]string{"order": "xx"}); r.Status != http.StatusBadRequest {
		t.Fatalf("sai mã: %d", r.Status)
	}
	if r := e.do(t, http.MethodPut, "/api/v1/settings/photo-order", kt.AccessToken, map[string]string{"order": "time"}); r.Status != http.StatusForbidden {
		t.Fatalf("kế toán sửa: %d", r.Status)
	}
	if r := e.do(t, http.MethodPut, "/api/v1/settings/photo-order", admin.AccessToken, map[string]string{"order": "time"}); r.Status != http.StatusOK {
		t.Fatalf("lưu quy tắc: %d", r.Status)
	}
	r = e.do(t, http.MethodGet, "/api/v1/settings/ai-rules", admin.AccessToken, nil)
	_ = json.Unmarshal(r.Body.Data, &rules)
	if rules.PhotoOrder != "time" {
		t.Fatalf("quy tắc sau khi lưu: %q", rules.PhotoOrder)
	}
	if r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), admin.AccessToken, nil); r.Status != http.StatusOK {
		t.Fatalf("xem hồ sơ sau khi đổi quy tắc: %d", r.Status)
	}
}

// Đồng bộ form Lark: nhà thầu đã tích "Xác minh tham gia" tự có tài khoản app (đăng nhập bằng SĐT, hồ sơ công ty điền sẵn);
// chưa xác minh thì không có tài khoản, đăng nhập "Nhà thầu" được báo đang xác minh; SĐT của CTV không bị đổi vai trò;
// đồng bộ lại không tạo trùng, không ghi đè tên công ty nhà thầu đã tự sửa.
func TestContractorLarkAccounts(t *testing.T) {
	t.Parallel()
	fl := &fakeLark{}
	e := newTestEnv(t, func(s *envSettings) { s.ntLark = fl })
	verified, pending, ctvPhone := e.newPhone(), e.newPhone(), e.newPhone()
	ctv := e.register(t, "CTV có SĐT trùng form")
	_ = ctv
	if _, err := e.db.Exec(context.Background(), `UPDATE users SET username = $1 WHERE id = $2`, ctvPhone, ctv.User.ID); err != nil {
		t.Fatal(err)
	}
	fl.recs = []lark.Record{
		larkRec(t, `{"record_id":"a1","fields":{"Tên công ty / Cá nhân":"Công ty Smartcons","Số điện thoại (có Zalo)":"`+verified+`",
			"Họ tên người liên hệ":"Anh Hưng","Tỉnh/thành phố nhận thi công":"TP. Hồ Chí Minh","Xác minh tham gia":true}}`),
		larkRec(t, `{"record_id":"a2","fields":{"Tên công ty / Cá nhân":"Tổ đội Minh","Số điện thoại (có Zalo)":"`+pending+`"}}`),
		larkRec(t, `{"record_id":"a3","fields":{"Tên công ty / Cá nhân":"Trùng CTV","Số điện thoại (có Zalo)":"`+ctvPhone+`","Xác minh tham gia":true}}`),
	}
	ctx := context.Background()
	if _, err := e.nt.SyncLark(ctx); err != nil {
		t.Fatal(err)
	}
	phoneLogin := func(p string) apiResp {
		t.Helper()
		return e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": p, "as": "nhathau"})
	}
	// đã xác minh: vào được, hồ sơ công ty điền sẵn, có nhãn đã xác minh
	r := phoneLogin(verified)
	expectStatus(t, r, http.StatusOK)
	var lg struct {
		Registered  bool       `json:"registered"`
		AccessToken string     `json:"access_token"`
		User        model.User `json:"user"`
	}
	decodeData(t, r, &lg)
	if !lg.Registered || lg.User.Role != model.RoleNhaThau || lg.User.Name != "Anh Hưng" {
		t.Fatalf("nhà thầu đã xác minh: %s", r.Raw)
	}
	var home struct {
		Profile struct {
			Company  string `json:"company"`
			Regions  string `json:"regions"`
			Verified bool   `json:"verified"`
		} `json:"profile"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", lg.AccessToken, nil), &home)
	if home.Profile.Company != "Công ty Smartcons" || home.Profile.Regions != "TP. Hồ Chí Minh" || !home.Profile.Verified {
		t.Fatalf("hồ sơ công ty: %+v", home.Profile)
	}
	// chưa xác minh: chưa có tài khoản, báo đang xác minh
	r = phoneLogin(pending)
	var pd struct {
		Registered bool `json:"registered"`
		NTPending  bool `json:"nt_pending"`
	}
	decodeData(t, r, &pd)
	if pd.Registered || !pd.NTPending {
		t.Fatalf("chưa xác minh: %s", r.Raw)
	}
	// SĐT của CTV: vẫn là CTV
	expectStatus(t, phoneLogin(ctvPhone), http.StatusConflict)

	// nhà thầu tự sửa tên công ty; đồng bộ lại không ghi đè, không tạo trùng
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/nt/profile", lg.AccessToken, map[string]any{"company": "Smartcons Việt Nam"}), http.StatusOK)
	// tích xác minh cho dòng thứ 2 trên Lark -> lượt sau có tài khoản
	fl.recs[1] = larkRec(t, `{"record_id":"a2","fields":{"Tên công ty / Cá nhân":"Tổ đội Minh","Số điện thoại (có Zalo)":"`+pending+`","Xác minh tham gia":true}}`)
	if _, err := e.nt.SyncLark(ctx); err != nil {
		t.Fatal(err)
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", lg.AccessToken, nil), &home)
	if home.Profile.Company != "Smartcons Việt Nam" {
		t.Fatalf("đồng bộ lại ghi đè tên công ty: %q", home.Profile.Company)
	}
	var n int
	_ = e.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'nhathau' AND username IN ($1, $2)`, verified, pending).Scan(&n)
	if n != 2 {
		t.Fatalf("tài khoản nhà thầu: %d", n)
	}
	decodeData(t, phoneLogin(pending), &lg)
	if !lg.Registered || lg.User.Name != "Tổ đội Minh" {
		t.Fatalf("vừa xác minh: %+v", lg)
	}

	// bỏ tích "Xác minh tham gia" của Smartcons trên Lark -> khoá, phiên đang mở hết hiệu lực, đăng nhập báo đang xác minh lại
	r = phoneLogin(verified)
	decodeData(t, r, &lg)
	oldTok := lg.AccessToken
	fl.recs[0] = larkRec(t, `{"record_id":"a1","fields":{"Tên công ty / Cá nhân":"Công ty Smartcons","Số điện thoại (có Zalo)":"`+verified+`","Họ tên người liên hệ":"Anh Hưng","Xác minh tham gia":false}}`)
	if _, err := e.nt.SyncLark(ctx); err != nil {
		t.Fatal(err)
	}
	if st := e.do(t, http.MethodGet, "/api/v1/nt/home", oldTok, nil).Status; st == http.StatusOK {
		t.Fatalf("phiên cũ vẫn dùng được sau khi khoá")
	}
	r = phoneLogin(verified)
	if r.Status == http.StatusOK || !strings.Contains(string(r.Raw), "xác minh lại") {
		t.Fatalf("bỏ tích: %d %s", r.Status, r.Raw)
	}
	// tích lại -> mở, dữ liệu vẫn còn
	fl.recs[0] = larkRec(t, `{"record_id":"a1","fields":{"Tên công ty / Cá nhân":"Công ty Smartcons","Số điện thoại (có Zalo)":"`+verified+`","Họ tên người liên hệ":"Anh Hưng","Xác minh tham gia":true}}`)
	if _, err := e.nt.SyncLark(ctx); err != nil {
		t.Fatal(err)
	}
	r = phoneLogin(verified)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &lg)
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", lg.AccessToken, nil), &home)
	if home.Profile.Company != "Smartcons Việt Nam" || !home.Profile.Verified {
		t.Fatalf("mở lại: %+v", home.Profile)
	}
	// cùng SĐT đăng ký thêm 1 dòng chưa tích: vẫn coi là đã xác minh, không khoá
	fl.recs = append(fl.recs, larkRec(t, `{"record_id":"a4","fields":{"Tên công ty / Cá nhân":"Smartcons (đăng ký lại)","Số điện thoại (có Zalo)":"`+verified+`"}}`))
	if _, err := e.nt.SyncLark(ctx); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, phoneLogin(verified), http.StatusOK)
	// admin khoá tay: đồng bộ (vẫn đang tích) không tự mở
	var uid int64
	_ = e.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, verified).Scan(&uid)
	if _, err := e.db.Exec(ctx, `UPDATE users SET locked = 1 WHERE id = $1`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.nt.SyncLark(ctx); err != nil {
		t.Fatal(err)
	}
	if r := phoneLogin(verified); r.Status == http.StatusOK {
		t.Fatalf("đồng bộ mở khoá của admin: %s", r.Raw)
	}
}
