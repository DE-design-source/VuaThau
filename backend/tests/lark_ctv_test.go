package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"vuathau/backend/internal/integration/lark"
)

type fakeLark struct{ recs []lark.Record }

func (f *fakeLark) Records(context.Context) ([]lark.Record, error) { return f.recs, nil }

func larkRec(t *testing.T, raw string) lark.Record {
	t.Helper()
	var r lark.Record
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// Form tuyển CTV trên Lark (góp ý 21/11/2026): dòng tích "Duyệt tham gia" -> tài khoản CTV (đăng nhập bằng SĐT),
// khu vực theo tỉnh trong form; chưa tích thì chưa tạo; đọc lại không tạo trùng; SĐT nhân viên bỏ qua.
func TestLarkCTVSync(t *testing.T) {
	t.Parallel()
	fl := &fakeLark{}
	e := newTestEnv(t, func(s *envSettings) { s.lark = fl })
	admin := e.staff(t, "admin")
	fl.recs = []lark.Record{
		// dạng ô chữ của Lark: mảng {type, text}
		larkRec(t, `{"record_id":"r1","fields":{"Họ và tên":[{"type":"text","text":"Hứa Văn Học"}],"Số điện thoại":"0985 361 339",
			"Tỉnh/thành phố bạn đang sống":"TP. Hồ Chí Minh","Công việc kinh nghiệm":"Kỹ sư xây dựng","Độ nhiệt tình":4,"Duyệt tham gia":true}}`),
		larkRec(t, `{"record_id":"r2","fields":{"Họ và tên":"Nguyễn Anh Thức","Số điện thoại":"0986286764","Duyệt tham gia":false}}`),
		larkRec(t, `{"record_id":"r3","fields":{"Họ và tên":"Không SĐT","Duyệt tham gia":true}}`),
	}
	ctx := context.Background()
	n, ok, err := e.user.SyncLarkCTVs(ctx)
	if err != nil || n != 1 || ok != 1 {
		t.Fatalf("đồng bộ Lark: tạo %d, duyệt %d, %v", n, ok, err)
	}
	var region, note, kind string
	if err := e.db.QueryRow(ctx, `SELECT region, note, kind FROM users WHERE username = '0985361339' AND role = 'ctv'`).Scan(&region, &note, &kind); err != nil {
		t.Fatalf("chưa tạo CTV: %v", err)
	}
	if region == "" || note == "" || kind != "CTV kinh doanh" {
		t.Fatalf("khu vực / ghi chú / chức năng: %q %q %q", region, note, kind)
	}
	var cnt int
	_ = e.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE username = '0986286764'`).Scan(&cnt)
	if cnt != 0 {
		t.Fatal("dòng chưa duyệt mà đã tạo CTV")
	}
	// đọc lại: không tạo trùng
	if n, _, err := e.user.SyncLarkCTVs(ctx); err != nil || n != 0 {
		t.Fatalf("đọc lại tạo thêm %d (%v)", n, err)
	}
	// CTV mới vào được app bằng SĐT, có trong danh sách "đã đăng ký"
	r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": "0985361339"})
	expectStatus(t, r, http.StatusOK)
	var d struct {
		Registered bool `json:"registered"`
	}
	decodeData(t, r, &d)
	if !d.Registered {
		t.Fatal("CTV từ Lark chưa đăng nhập được bằng SĐT")
	}
	var l struct {
		Rows []struct {
			Name string `json:"name"`
		} `json:"rows"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/ctvs?status=dang_ky&q=Hứa", admin.AccessToken, nil), &l)
	if len(l.Rows) != 1 {
		t.Fatalf("danh sách đã đăng ký: %+v", l.Rows)
	}
}
