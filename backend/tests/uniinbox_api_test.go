package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"vuathau/backend/internal/handler"
	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

const uniKey = "khoa-api-uniinbox-test-0123456789abcdef"

// fakeUniInbox: API đọc của UniInbox (GET /integrations/vuathau/*) đúng hợp đồng JSON bên đó.
func fakeUniInbox(t *testing.T, img []byte) *httptest.Server {
	t.Helper()
	log := map[string]any{
		"id": "aaaaaaaa-0000-4000-8000-000000000001", "conversation_id": "cccccccc-0000-4000-8000-000000000001",
		"created_at": "2026-11-25T07:00:00.000Z", "updated_at": "2026-11-25T07:00:00.123Z",
		"fields": []map[string]string{
			{"key": "dia_chi", "label": "Địa chỉ công trình", "value": "12 Lê Lợi, Huế"},
			{"key": "nha_thau", "label": "Nhà thầu", "value": "Xây dựng Thuận Phát"},
			{"key": "sdt_nha_thau", "label": "SĐT", "value": "0911222333"},
			{"key": "toa_do", "label": "Toạ độ", "value": "16.463713°N, 107.590866°E"},
		},
		"contact": map[string]any{"id": "dddddddd-0000-4000-8000-000000000001", "name": "Nguyễn Văn API", "phone": "0901234999",
			"profile_fields": map[string]any{"khu_vuc": map[string]string{"label": "Khu vực", "value": "Huế"}}},
		"images": []map[string]string{{"key": "media/a.jpg", "mime": "image/jpeg", "sent_at": "2026-11-25T06:59:00.000Z"}},
		"videos": []map[string]string{},
	}
	mux := http.NewServeMux()
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+uniKey {
				http.Error(w, `{"message":"Sai khoá API"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /integrations/vuathau/project-logs", auth(func(w http.ResponseWriter, r *http.Request) {
		items := []any{}
		if r.URL.Query().Get("after_id") == "" { // trang đầu có 1 hồ sơ, sau mốc thì hết
			items = append(items, log)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	mux.HandleFunc("GET /integrations/vuathau/photo-messages", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	mux.HandleFunc("GET /integrations/vuathau/contacts", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": map[string]any{
			"dddddddd-0000-4000-8000-000000000001": map[string]any{"name": "Nguyễn Văn API", "phone": "0901234999", "tags": []string{"CTV"},
				"profile_fields": map[string]any{"khu_vuc": map[string]string{"label": "Khu vực", "value": "Huế"}}, "updated_at": time.Now()},
		}})
	}))
	mux.HandleFunc("GET /integrations/vuathau/media", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "media/a.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(img)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Đồng bộ hồ sơ Messenger qua API của UniInbox (không đọc DB / kho S3 của UniInbox): tạo CTV + hồ sơ, ảnh tải về.
func TestInboxViaUniInboxAPI(t *testing.T) {
	t.Parallel()
	srv := fakeUniInbox(t, bigPhoto(t, 9900))
	if _, err := inbox.NewAPI(inbox.APIConfig{BaseURL: srv.URL, Key: "ngan"}); err == nil {
		t.Fatal("khoá API ngắn phải bị từ chối")
	}
	src, err := inbox.NewAPI(inbox.APIConfig{BaseURL: srv.URL, Key: uniKey})
	if err != nil {
		t.Fatal(err)
	}
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src })
	admin := e.staff(t, model.RoleAdmin)
	r := e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var st service.SyncStatus
	decodeData(t, r, &st)
	if st.Error != "" || st.Added != 1 {
		t.Fatalf("đồng bộ qua API: %+v", st)
	}
	var addr, ctv string
	var photos int
	if err := e.db.QueryRow(t.Context(), `SELECT h.address, u.name, (SELECT count(*) FROM photos p WHERE p.house_id = h.id)
		FROM houses h JOIN users u ON u.id = h.ctv_id WHERE h.ext_id = 'aaaaaaaa-0000-4000-8000-000000000001'`).Scan(&addr, &ctv, &photos); err != nil {
		t.Fatalf("chưa có hồ sơ: %v", err)
	}
	if !strings.Contains(addr, "Lê Lợi") || ctv != "Nguyễn Văn API" || photos != 1 {
		t.Fatalf("hồ sơ: %q %q %d ảnh", addr, ctv, photos)
	}
	// đồng bộ lại: không tạo trùng
	r = e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
	decodeData(t, r, &st)
	if st.Added != 0 {
		t.Fatalf("đồng bộ lại tạo thêm: %+v", st)
	}
	// khoá sai: báo lỗi rõ, không sập
	bad, _ := inbox.NewAPI(inbox.APIConfig{BaseURL: srv.URL, Key: strings.Repeat("x", 40)})
	if _, err := bad.Logs(t.Context(), time.Time{}, "", 10); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("khoá sai: %v", err)
	}
}

// Lời báo của UniInbox: đúng chữ ký thì đồng bộ ngay (202); sai chữ ký / quá cũ: 401; chưa cấu hình khoá: 404.
func TestInboxNotify(t *testing.T) {
	t.Parallel()
	const secret = "khoa-bao-uniinbox-test-0123456789abcdef"
	kicks := 0
	h := handler.NewInboxNotifyHandler(secret, func() { kicks++ })
	send := func(h *handler.InboxNotifyHandler, ts int64, body, sig string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/uniinbox/notify", strings.NewReader(body))
		req.Header.Set("X-UniInbox-Timestamp", strconv.FormatInt(ts, 10))
		req.Header.Set("X-UniInbox-Signature", sig)
		w := httptest.NewRecorder()
		h.Notify(w, req)
		return w.Code
	}
	sign := func(ts int64, body string) string {
		m := hmac.New(sha256.New, []byte(secret))
		fmt.Fprintf(m, "%d.%s", ts, body)
		return "sha256=" + hex.EncodeToString(m.Sum(nil))
	}
	now := time.Now().Unix()
	body := `{"event":"project_logs.created","ids":["a"]}`
	if c := send(h, now, body, sign(now, body)); c != http.StatusAccepted || kicks != 1 {
		t.Fatalf("lời báo đúng: %d, kick %d", c, kicks)
	}
	if c := send(h, now, body, sign(now, body+"x")); c != http.StatusUnauthorized {
		t.Fatalf("sai chữ ký: %d", c)
	}
	old := now - 600
	if c := send(h, old, body, sign(old, body)); c != http.StatusUnauthorized {
		t.Fatalf("lời báo cũ: %d", c)
	}
	if c := send(handler.NewInboxNotifyHandler("", nil), now, body, sign(now, body)); c != http.StatusNotFound {
		t.Fatalf("chưa cấu hình: %d", c)
	}
	if kicks != 1 {
		t.Fatalf("lời báo sai vẫn kick: %d", kicks)
	}
}
