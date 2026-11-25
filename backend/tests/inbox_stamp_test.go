package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/integration/ocr"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

// fakeStamp: "đọc" dải chữ ảnh theo sha256 của ảnh; không che SĐT (Enabled false).
type fakeStamp map[string]string

func (fakeStamp) Enabled() bool                                               { return false }
func (fakeStamp) Phones(context.Context, []byte, []string) ([]ocr.Box, error) { return nil, nil }
func (f fakeStamp) Stamp(_ context.Context, b []byte) (string, error) {
	h := sha256.Sum256(b)
	return f[hex.EncodeToString(h[:])], nil
}

// CTV gửi liền 2 công trình (cách nhau ~2 km) trong 1 đợt: đọc toạ độ in trên ảnh, tách 2 hồ sơ; ảnh có toạ độ thì
// có vị trí, ảnh không đọc được toạ độ theo công trình đang gom.
func TestInboxLooseSplitByStamp(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Truncate(time.Millisecond).Format(time.RFC3339Nano) }
	contact := map[string]any{"id": "eeeeeeee-0000-4000-8000-0000000000aa", "name": "Vinh Thử", "phone": "", "profile_fields": map[string]any{}}
	msg := func(id, sent string, keys ...string) map[string]any {
		imgs := []map[string]string{}
		for _, k := range keys {
			imgs = append(imgs, map[string]string{"key": k, "mime": "image/jpeg"})
		}
		return map[string]any{"id": id, "conversation_id": "cccccccc-0000-4000-8000-0000000000aa", "sent_at": sent,
			"in_project_log": false, "contact": contact, "images": imgs}
	}
	msgs := []map[string]any{
		msg("abababab-0000-4000-8000-000000000001", at(6*time.Hour), "s/1.jpg", "s/2.jpg"),
		msg("abababab-0000-4000-8000-000000000002", at(6*time.Hour-10*time.Minute), "s/3.jpg", "s/4.jpg"),
	}
	imgs, stamps := map[string][]byte{}, fakeStamp{}
	texts := map[string]string{
		"s/1.jpg": "Thứ Tư, 25 Tháng 11, 2026 13:22\n81 Đường Số 7\n10.799347°N, 106.736523°E",
		"s/2.jpg": "10.799390°N,106.736436°E",                                 // cùng công trình (~10 m)
		"s/3.jpg": "Thứ Tư 25 Tháng 11 2026 13:52\n10.830610°N, 106.759187°F", // công trình khác (~4 km), OCR đọc E thành F
		"s/4.jpg": "chữ nhiễu không có toạ độ",
	}
	for i, k := range []string{"s/1.jpg", "s/2.jpg", "s/3.jpg", "s/4.jpg"} {
		b := bigPhoto(t, uint64(8800+i))
		imgs[k] = b
		h := sha256.Sum256(b)
		stamps[hex.EncodeToString(h[:])] = texts[k]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /integrations/vuathau/photo-messages", func(w http.ResponseWriter, r *http.Request) {
		items := []any{}
		if r.URL.Query().Get("after_id") == "" {
			for _, m := range msgs {
				items = append(items, m)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	mux.HandleFunc("GET /integrations/vuathau/project-logs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})
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
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src; s.ocr = stamps })
	admin := e.staff(t, model.RoleAdmin)
	r := e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var st service.SyncStatus
	decodeData(t, r, &st)
	if st.Error != "" || st.Added != 2 {
		t.Fatalf("phải tách 2 hồ sơ: %+v", st)
	}
	rows, err := e.db.Query(t.Context(), `SELECT h.ext_id, count(p.id), count(p.lat), h.lat IS NOT NULL
		FROM houses h JOIN photos p ON p.house_id = h.id WHERE h.ext_id LIKE 'msg:%' GROUP BY h.id ORDER BY h.ext_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		ext          string
		photos, gps  int
		houseLocated bool
	}
	got := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ext, &x.photos, &x.gps, &x.houseLocated); err != nil {
			t.Fatal(err)
		}
		got = append(got, x)
	}
	want := []row{
		{"msg:abababab-0000-4000-8000-000000000001", 2, 2, true},
		{"msg:abababab-0000-4000-8000-000000000001#2", 2, 1, true},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hồ sơ: %+v, muốn %+v", got, want)
	}
}
