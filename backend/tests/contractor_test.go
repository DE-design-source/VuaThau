package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"vuathau/backend/internal/integration/lark"
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
}
