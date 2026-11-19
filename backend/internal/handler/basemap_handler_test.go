package handler

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBasemapStyle(t *testing.T) {
	var hits atomic.Int32
	down := atomic.Bool{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if down.Load() {
			http.Error(w, "x", http.StatusBadGateway)
			return
		}
		if r.URL.Query().Get("apikey") != "k1" {
			http.Error(w, "bad key", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"version":8}`))
	}))
	defer up.Close()

	get := func(h *BasemapHandler) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.Style(rec, httptest.NewRequest(http.MethodGet, "/map-style/vn.json", nil))
		return rec
	}

	// chưa có key: 404 (web dùng nền cũ)
	if rec := get(NewBasemapHandler("")); rec.Code != http.StatusNotFound {
		t.Fatalf("no key: %d", rec.Code)
	}

	h := NewBasemapHandler("k1").WithStyleURL(up.URL + "/style.json?apikey=%s")
	rec := get(h)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"version":8}` || rec.Header().Get("Cache-Control") == "" {
		t.Fatalf("first: %d %q", rec.Code, rec.Body.String())
	}
	// lần 2: lấy từ bản đã lưu, không gọi Vietmap
	get(h)
	if hits.Load() != 1 {
		t.Fatalf("cache: %d hits", hits.Load())
	}
	// hết hạn + Vietmap lỗi: vẫn trả bản đã lưu
	h.at = h.at.Add(-2 * vietmapKeep)
	down.Store(true)
	if rec := get(h); rec.Code != http.StatusOK {
		t.Fatalf("stale: %d", rec.Code)
	}
	// key sai, chưa có bản lưu: 502
	if rec := get(NewBasemapHandler("sai").WithStyleURL(up.URL + "/style.json?apikey=%s")); rec.Code != http.StatusBadGateway {
		t.Fatalf("bad key: %d", rec.Code)
	}
}
