package handler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ô nền bản đồ (OpenStreetMap) đi qua máy chủ mình: GET /tiles/{z}/{x}/{y}.png
//
// Trình duyệt gọi thẳng tile.openstreetmap.org hay bị chặn (trình chặn quảng cáo bỏ Referer, mạng công ty, bị bóp
// theo IP) và chậm (máy chủ OSM ở Singapore). Ở đây máy chủ lấy giùm theo đúng quy định của OSM (User-Agent nêu tên
// app, tối đa 2 kết nối cùng lúc), lưu vào DATA_DIR/tiles 30 ngày; người sau xem lại lấy từ máy chủ mình, rất nhanh.
// Quy định: https://operations.osmfoundation.org/policies/tiles/

const (
	tileUpstream = "https://tile.openstreetmap.org/%d/%d/%d.png"
	tileUA       = "VuaThauNghiemThu/1.0 (+https://map.vuathau.com.vn)"
	tileMaxZoom  = 19
	tileKeep     = 30 * 24 * time.Hour // giữ ô đã lưu bao lâu trước khi lấy lại
	tileBrowser  = "public, max-age=604800"
)

type TileHandler struct {
	dir      string
	upstream string // đổi được khi test
	client   *http.Client
	slots    chan struct{} // tối đa 2 kết nối tới OSM cùng lúc
	mu       sync.Mutex
	inflight map[string]*tileCall // nhiều người cùng xem 1 ô: chỉ lấy 1 lần
}

type tileCall struct {
	done chan struct{}
	data []byte
	err  error
}

func NewTileHandler(dataDir string) *TileHandler {
	return &TileHandler{
		dir:      filepath.Join(dataDir, "tiles"),
		upstream: tileUpstream,
		client:   &http.Client{Timeout: 20 * time.Second},
		slots:    make(chan struct{}, 2),
		inflight: map[string]*tileCall{},
	}
}

// WithUpstream: dùng trong test (máy chủ giả thay OSM).
func (h *TileHandler) WithUpstream(u string) *TileHandler {
	h.upstream = u
	return h
}

// parseTile: z, x, y hợp lệ (0 ≤ x, y < 2^z, z ≤ 19).
func parseTile(zs, xs, file string) (z, x, y int, ok bool) {
	ys, found := strings.CutSuffix(file, ".png")
	if !found {
		return 0, 0, 0, false
	}
	var err error
	if z, err = strconv.Atoi(zs); err != nil || z < 0 || z > tileMaxZoom {
		return 0, 0, 0, false
	}
	n := 1 << z
	if x, err = strconv.Atoi(xs); err != nil || x < 0 || x >= n {
		return 0, 0, 0, false
	}
	if y, err = strconv.Atoi(ys); err != nil || y < 0 || y >= n {
		return 0, 0, 0, false
	}
	return z, x, y, true
}

func (h *TileHandler) Serve(w http.ResponseWriter, r *http.Request) {
	z, x, y, ok := parseTile(r.PathValue("z"), r.PathValue("x"), r.PathValue("file"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(h.dir, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
	st, err := os.Stat(path)
	if err == nil && time.Since(st.ModTime()) < tileKeep {
		h.write(w, r, path, nil)
		return
	}
	data, ferr := h.fetch(r.Context(), z, x, y, path)
	if ferr != nil {
		if err == nil { // OSM lỗi nhưng còn bản cũ: dùng tạm
			h.write(w, r, path, nil)
			return
		}
		log.Printf("tile %d/%d/%d: %v", z, x, y, ferr)
		http.Error(w, "Không tải được bản đồ", http.StatusBadGateway)
		return
	}
	h.write(w, r, "", data)
}

func (h *TileHandler) write(w http.ResponseWriter, r *http.Request, path string, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", tileBrowser)
	if data == nil {
		http.ServeFile(w, r, path)
		return
	}
	_, _ = w.Write(data)
}

func (h *TileHandler) fetch(ctx context.Context, z, x, y int, path string) ([]byte, error) {
	h.mu.Lock()
	if c, ok := h.inflight[path]; ok {
		h.mu.Unlock()
		select {
		case <-c.done:
			return c.data, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &tileCall{done: make(chan struct{})}
	h.inflight[path] = c
	h.mu.Unlock()

	// Không gắn với request của người xem: họ đóng trang thì ô vẫn lấy xong và lưu cho lần sau
	c.data, c.err = h.download(z, x, y, path)
	close(c.done)
	h.mu.Lock()
	delete(h.inflight, path)
	h.mu.Unlock()
	return c.data, c.err
}

func (h *TileHandler) download(z, x, y int, path string) ([]byte, error) {
	h.slots <- struct{}{}
	defer func() { <-h.slots }()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(h.upstream, z, x, y), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", tileUA)
	req.Header.Set("Referer", "https://map.vuathau.com.vn/")
	res, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/") {
		return nil, fmt.Errorf("OSM trả %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	// Lưu: ghi file tạm rồi đổi tên, để người khác không đọc phải file đang ghi dở
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		tmp := path + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 36)
		if err := os.WriteFile(tmp, data, 0o644); err == nil {
			if err := os.Rename(tmp, path); err != nil {
				_ = os.Remove(tmp)
			}
		}
	}
	return data, nil
}
