package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTileProxy(t *testing.T) {
	var hits atomic.Int32
	var ua, ref string
	fail := atomic.Bool{}
	osm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		ua, ref = r.UserAgent(), r.Referer()
		if fail.Load() {
			http.Error(w, "blocked", http.StatusForbidden)
			return
		}
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG:" + r.URL.Path))
	}))
	defer osm.Close()

	dir := t.TempDir()
	h := NewTileHandler(dir).WithUpstream(osm.URL + "/%d/%d/%d.png")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tiles/{z}/{x}/{file}", h.Serve)
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		return w
	}

	// 5 người cùng mở 1 ô: chỉ gọi OSM 1 lần
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := get("/tiles/6/50/28.png"); w.Code != 200 || w.Body.String() != "PNG:/6/50/28.png" {
				t.Errorf("tile: %d %q", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("OSM bị gọi %d lần, muốn 1", hits.Load())
	}
	if ua != tileUA || ref == "" {
		t.Fatalf("thiếu User-Agent / Referer theo quy định OSM: %q %q", ua, ref)
	}

	// Lần sau: lấy từ ổ đĩa, không gọi OSM; trình duyệt được lưu đệm
	w := get("/tiles/6/50/28.png")
	if w.Code != 200 || hits.Load() != 1 || w.Header().Get("Cache-Control") != tileBrowser || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("cache: %d hits=%d %v", w.Code, hits.Load(), w.Header())
	}

	// Ô sai: 404, không gọi OSM
	for _, p := range []string{"/tiles/20/0/0.png", "/tiles/2/4/0.png", "/tiles/2/0/-1.png", "/tiles/2/0/0.jpg", "/tiles/a/0/0.png"} {
		if w := get(p); w.Code != 404 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("ô sai vẫn gọi OSM")
	}

	// OSM chặn: ô chưa có thì báo lỗi; ô đã có (dù cũ) vẫn dùng tạm
	fail.Store(true)
	if w := get("/tiles/7/100/56.png"); w.Code != http.StatusBadGateway {
		t.Fatalf("OSM lỗi, ô chưa có: %d", w.Code)
	}
	old := time.Now().Add(-2 * tileKeep)
	_ = os.Chtimes(filepath.Join(dir, "tiles", "6", "50", "28.png"), old, old)
	if w := get("/tiles/6/50/28.png"); w.Code != 200 || w.Body.String() != "PNG:/6/50/28.png" {
		t.Fatalf("OSM lỗi, dùng bản cũ: %d %q", w.Code, w.Body.String())
	}
}
