package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Nền bản đồ Vietmap cho web quản lý: GET /map-style/vn.json
//
// Bản đồ của công ty Việt Nam: Hoàng Sa, Trường Sa ghi đúng thuộc Việt Nam (OpenStreetMap thì không).
// Key nằm trong .env của máy chủ (VIETMAP_API_KEY), không nằm trong mã web. Máy chủ lấy kiểu bản đồ (style.json)
// của Vietmap, giữ 6 giờ; trong đó các ô bản đồ / phông chữ đã kèm key nên trình duyệt tải thẳng từ Vietmap
// (key Vietmap dùng phía trình duyệt là bình thường: giới hạn theo tên miền trong trang quản lý của Vietmap).
// Chưa có key: 404, web dùng nền cũ (/map-style.json).

const (
	vietmapStyleURL = "https://maps.vietmap.vn/maps/styles/tm/style.json?apikey=%s"
	vietmapKeep     = 6 * time.Hour
)

type BasemapHandler struct {
	key      string
	styleURL string // đổi được khi test
	client   *http.Client
	mu       sync.Mutex
	style    []byte
	at       time.Time
}

func NewBasemapHandler(vietmapKey string) *BasemapHandler {
	return &BasemapHandler{key: vietmapKey, styleURL: vietmapStyleURL, client: &http.Client{Timeout: 15 * time.Second}}
}

// WithStyleURL: dùng trong test (máy chủ giả thay Vietmap); %s là chỗ điền key.
func (h *BasemapHandler) WithStyleURL(u string) *BasemapHandler {
	h.styleURL = u
	return h
}

func (h *BasemapHandler) Style(w http.ResponseWriter, r *http.Request) {
	if h.key == "" {
		http.Error(w, "chưa cấu hình VIETMAP_API_KEY", http.StatusNotFound)
		return
	}
	b, err := h.get(r.Context())
	if err != nil {
		http.Error(w, "không lấy được bản đồ Vietmap", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(b)
}

func (h *BasemapHandler) get(ctx context.Context) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.style != nil && time.Since(h.at) < vietmapKeep {
		return h.style, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(h.styleURL, url.QueryEscape(h.key)), nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return h.stale(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return h.stale(err)
	}
	if resp.StatusCode != http.StatusOK || len(b) == 0 || b[0] != '{' {
		return h.stale(fmt.Errorf("vietmap style: %d", resp.StatusCode))
	}
	h.style, h.at = b, time.Now()
	return b, nil
}

// stale: Vietmap lỗi thì dùng tạm bản đã lưu (nếu có).
func (h *BasemapHandler) stale(err error) ([]byte, error) {
	if h.style != nil {
		return h.style, nil
	}
	return nil, err
}
