package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

// Ảnh CTV gửi trên fanpage mà chatbot không ghi nhật ký: gom tin liền nhau (≤30 phút) của 1 người thành 1 hồ sơ
// "Chờ phân loại" trong hàng chờ duyệt; tin thuộc nhật ký bỏ qua; tin mới (chưa đủ 3 giờ) và nhóm chưa khép thì chờ.
func TestInboxLoosePhotos(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Truncate(time.Millisecond).Format(time.RFC3339Nano) }
	contact := map[string]any{"id": "eeeeeeee-0000-4000-8000-000000000001", "name": "Trần Văn Sót", "phone": "0901234777",
		"profile_fields": map[string]any{}}
	other := map[string]any{"id": "eeeeeeee-0000-4000-8000-000000000002", "name": "Lê Thị Khác", "phone": "",
		"profile_fields": map[string]any{}}
	msg := func(id, sent string, c map[string]any, inLog bool, keys ...string) map[string]any {
		imgs := []map[string]string{}
		for _, k := range keys {
			imgs = append(imgs, map[string]string{"key": k, "mime": "image/jpeg"})
		}
		return map[string]any{"id": id, "conversation_id": "cccccccc-0000-4000-8000-0000000000" + id[len(id)-2:],
			"sent_at": sent, "in_project_log": inLog, "contact": c, "images": imgs}
	}
	msgs := []map[string]any{
		msg("ffffffff-0000-4000-8000-000000000001", at(10*time.Hour), contact, false, "m/1.jpg"),
		msg("ffffffff-0000-4000-8000-000000000002", at(10*time.Hour-10*time.Minute), other, false, "m/2.jpg"),
		msg("ffffffff-0000-4000-8000-000000000003", at(10*time.Hour-20*time.Minute), contact, false, "m/3.jpg", "m/4.jpg"),
		// cách 2 giờ: đợt khác
		msg("ffffffff-0000-4000-8000-000000000004", at(8*time.Hour), contact, false, "m/5.jpg"),
		// tin đã thuộc nhật ký: bỏ qua, ngắt nhóm
		msg("ffffffff-0000-4000-8000-000000000005", at(8*time.Hour-5*time.Minute), contact, true, "m/6.jpg"),
		msg("ffffffff-0000-4000-8000-000000000006", at(8*time.Hour-10*time.Minute), contact, false, "m/7.jpg"),
		// nhóm chưa khép (tin cuối cách mốc 3 giờ chưa đủ 30 phút) và tin mới: chờ lượt sau
		msg("ffffffff-0000-4000-8000-000000000007", at(3*time.Hour+10*time.Minute), other, false, "m/8.jpg"),
		msg("ffffffff-0000-4000-8000-000000000008", at(time.Hour), other, false, "m/9.jpg"),
	}
	imgs := map[string][]byte{}
	for i, k := range []string{"m/1.jpg", "m/2.jpg", "m/3.jpg", "m/4.jpg", "m/5.jpg", "m/6.jpg", "m/7.jpg", "m/8.jpg", "m/9.jpg"} {
		imgs[k] = bigPhoto(t, uint64(7100+i))
	}
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /integrations/vuathau/photo-messages", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if q.Get("since") == "" || r.Header.Get("Authorization") != "Bearer "+uniKey {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		items := []any{}
		for _, m := range msgs {
			if q.Get("after_id") == "" || m["sent_at"].(string) > q.Get("after_at") ||
				(m["sent_at"].(string) == q.Get("after_at") && m["id"].(string) > q.Get("after_id")) {
				items = append(items, m)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	for _, p := range []string{"project-logs"} {
		mux.HandleFunc("GET /integrations/vuathau/"+p, func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
		})
	}
	mux.HandleFunc("GET /integrations/vuathau/contacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": map[string]any{}})
	})
	mux.HandleFunc("GET /integrations/vuathau/media", func(w http.ResponseWriter, r *http.Request) {
		b, ok := imgs[r.URL.Query().Get("key")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	src, err := inbox.NewAPI(inbox.APIConfig{BaseURL: srv.URL, Key: uniKey})
	if err != nil {
		t.Fatal(err)
	}
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src })
	admin := e.staff(t, model.RoleAdmin)
	sync := func() service.SyncStatus {
		r := e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
		expectStatus(t, r, http.StatusOK)
		var st service.SyncStatus
		decodeData(t, r, &st)
		if st.Error != "" {
			t.Fatalf("đồng bộ: %+v", st)
		}
		return st
	}
	if st := sync(); st.Added != 4 {
		t.Fatalf("phải tạo 4 hồ sơ chờ phân loại (tin 1+3, tin 2, tin 4, tin 6): %+v", st)
	}
	type row struct {
		ext, status, dup, ctv string
		photos                int
	}
	rows, err := e.db.Query(t.Context(), `SELECT h.ext_id, h.status, h.dup, u.name, (SELECT count(*) FROM photos p WHERE p.house_id = h.id)
		FROM houses h JOIN users u ON u.id = h.ctv_id WHERE h.ext_id LIKE 'msg:%' ORDER BY h.ext_id`)
	if err != nil {
		t.Fatal(err)
	}
	got := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ext, &x.status, &x.dup, &x.ctv, &x.photos); err != nil {
			t.Fatal(err)
		}
		got = append(got, x)
	}
	rows.Close()
	want := []struct {
		id     string
		photos int
		ctv    string
	}{{"01", 3, "Trần Văn Sót"}, {"02", 1, "Lê Thị Khác"}, {"04", 1, "Trần Văn Sót"}, {"06", 1, "Trần Văn Sót"}}
	if len(got) != len(want) {
		t.Fatalf("hồ sơ: %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if g.ext != "msg:ffffffff-0000-4000-8000-0000000000"+w.id || g.photos != w.photos || g.ctv != w.ctv ||
			g.status != model.StatusPending || !strings.HasPrefix(g.dup, service.LooseLabel) {
			t.Fatalf("hồ sơ %d: %+v (muốn %+v)", i, g, w)
		}
	}
	// lượt sau: không tạo lại; nhóm chưa khép vẫn chờ
	if st := sync(); st.Added != 0 {
		t.Fatalf("đồng bộ lại tạo thêm: %+v", st)
	}
	var n int
	if err := e.db.QueryRow(t.Context(), `SELECT count(*) FROM houses WHERE ext_id LIKE 'msg:%'`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("số hồ sơ chờ phân loại: %d %v", n, err)
	}
	// admin điền địa chỉ: ảnh không GPS lấy vị trí theo địa chỉ (hết "Không có vị trí"), bỏ nhãn chờ phân loại, (tiền tính lại theo ảnh đạt)
	var hid int64
	var amt0 int64
	if err := e.db.QueryRow(t.Context(), `SELECT id, amount FROM houses WHERE ext_id = 'msg:ffffffff-0000-4000-8000-000000000001'`).Scan(&hid, &amt0); err != nil {
		t.Fatal(err)
	}
	r := e.do(t, http.MethodPut, "/api/v1/houses/"+strconv.FormatInt(hid, 10)+"/info", admin.AccessToken,
		map[string]any{"address": "25 Trần Hưng Đạo, Phường Phú Hội, Thành phố Huế"})
	expectStatus(t, r, http.StatusOK)
	// 3 ảnh đạt < số ảnh tối thiểu: tiền vẫn như cũ
	var withAddr, dup string
	var amt1 int64
	if err := e.db.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM photos p WHERE p.house_id = h.id AND p.address <> ''), h.dup, h.amount
		FROM houses h WHERE h.id = $1`, hid).Scan(&withAddr, &dup, &amt1); err != nil {
		t.Fatal(err)
	}
	if withAddr != "3" || dup != "" || amt1 != amt0 {
		t.Fatalf("sau khi điền địa chỉ: %s ảnh có vị trí, dup %q, tiền %d -> %d", withAddr, dup, amt0, amt1)
	}
	if calls.Load() < 2 {
		t.Fatalf("không gọi API tin có ảnh")
	}
}
