package tests

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"vuathau/backend/internal/integration/ai"
	"vuathau/backend/internal/model"
)

// fakeAI: AI giả, ghi lại ảnh nhận được (kiểm ảnh gửi đi là JPEG đã thu nhỏ) và trả kết quả cố định.
type fakeAI struct {
	mu    sync.Mutex
	calls int
	sizes []int  // cạnh dài từng ảnh nhận
	bad   int    // ảnh không đọc được
	rules string // quy tắc admin gửi kèm lượt gần nhất
}

func (f *fakeAI) Enabled() bool { return true }

func (f *fakeAI) ReadPhotos(ctx context.Context, images [][]byte) (*ai.Read, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.rules = ai.RulesFrom(ctx)
	r := &ai.Read{ContractorName: "CÔNG TY XÂY DỰNG AI ĐỌC", ContractorPhone: "0909123456", Floors: 3, Stage: model.Stages[3]}
	for i, b := range images {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			f.bad++
		} else {
			f.sizes = append(f.sizes, max(cfg.Width, cfg.Height))
		}
		r.Photos = append(r.Photos, ai.PhotoRead{Index: i, IsSite: true, Inside: i == 0})
	}
	return r, nil
}

// AI đọc ảnh vẫn chạy sau bản tăng tốc nộp ảnh (19/11): kiểm ảnh lúc chụp gọi AI; nộp hồ sơ xong AI đọc nền,
// ảnh thu nhỏ (tạo lúc AI cần) là JPEG <= 1568px; kết quả điền vào hồ sơ (nhà thầu, giai đoạn, gợi ý loại chụp).
func TestAIFlow(t *testing.T) {
	t.Parallel()
	fa := &fakeAI{}
	e := newTestEnv(t, func(s *envSettings) { s.ai = fa })
	ctv := e.register(t, "CTV có AI")
	ctx := context.Background()

	files := make([][]byte, 6)
	for i := range files {
		files[i] = bigPhoto(t, 8800+uint64(i))
	}
	// kiểm từng ảnh lúc chụp: AI trả kết quả (ai: true)
	for _, f := range files {
		r := e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{f}, nil)
		expectStatus(t, r, http.StatusOK)
		var c struct {
			AI bool `json:"ai"`
		}
		decodeData(t, r, &c)
		if !c.AI {
			t.Fatalf("kiểm ảnh: AI không chạy: %s", r.Raw)
		}
	}
	checkCalls := fa.calls

	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(6, 10.42)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	hid := *up.HouseID

	// AI đọc nền: chờ xong
	var state, name, phone, scopeAI string
	deadline := time.Now().Add(20 * time.Second)
	for {
		if err := e.db.QueryRow(ctx, `SELECT ai_state, contractor_name, contractor_phone, scope_ai FROM houses WHERE id = $1`, hid).
			Scan(&state, &name, &phone, &scopeAI); err != nil {
			t.Fatal(err)
		}
		if state == model.AIDone || state == model.AIFailed || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if state != model.AIDone {
		t.Fatalf("AI hồ sơ: trạng thái %q", state)
	}
	if name != "CÔNG TY XÂY DỰNG AI ĐỌC" || phone != "0909123456" || scopeAI != model.ScopeBoth {
		t.Fatalf("kết quả AI: %q %q %q", name, phone, scopeAI)
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.bad > 0 {
		t.Fatalf("AI nhận %d ảnh không đọc được", fa.bad)
	}
	for _, s := range fa.sizes {
		if s > 1568 {
			t.Fatalf("ảnh gửi AI quá lớn: %dpx", s)
		}
	}
	t.Logf("AI: %d lượt lúc kiểm ảnh, %d lượt tổng; ảnh gửi AI cạnh dài %v", checkCalls, fa.calls, fa.sizes[:min(3, len(fa.sizes))])

	// xem ảnh lần đầu: tạo ảnh thu nhỏ (JPEG <= 1568px) và lưu lại cho lần sau
	var pid int64
	var sha string
	_ = e.db.QueryRow(ctx, `SELECT id, sha256 FROM photos WHERE house_id = $1 ORDER BY id LIMIT 1`, hid).Scan(&pid, &sha)
	g := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", pid), ctv.AccessToken)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(g.Raw))
	if g.Status != http.StatusOK || err != nil || max(cfg.Width, cfg.Height) != 1568 {
		t.Fatalf("xem ảnh: %d %v %dx%d", g.Status, err, cfg.Width, cfg.Height)
	}
	if _, err := os.Stat(filepath.Join(e.data, "anh", sha+"_s.jpg")); err != nil {
		t.Fatal("ảnh thu nhỏ chưa được lưu")
	}

	// ảnh chọn từ thư viện (không kiểm lúc chụp): AI đọc nền, tự tạo ảnh thu nhỏ từ ảnh gốc
	before := len(fa.sizes)
	fa.mu.Unlock()
	lib := make([][]byte, 5)
	for i := range lib {
		lib[i] = bigPhoto(t, 8900+uint64(i))
	}
	r = e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", lib, map[string]string{"meta": cameraMeta(5, 10.47)})
	expectStatus(t, r, http.StatusCreated)
	decodeData(t, r, &up)
	deadline = time.Now().Add(20 * time.Second)
	for {
		_ = e.db.QueryRow(ctx, `SELECT ai_state FROM houses WHERE id = $1`, *up.HouseID).Scan(&state)
		if state == model.AIDone || state == model.AIFailed || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	fa.mu.Lock()
	if state != model.AIDone || len(fa.sizes)-before != 5 || fa.bad > 0 {
		t.Fatalf("ảnh thư viện: AI %q, nhận %d ảnh, %d ảnh lỗi", state, len(fa.sizes)-before, fa.bad)
	}
	for _, s := range fa.sizes[before:] {
		if s != 1568 {
			t.Fatalf("ảnh thư viện gửi AI cạnh dài %d", s)
		}
	}
}
