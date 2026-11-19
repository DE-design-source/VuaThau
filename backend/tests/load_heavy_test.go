package tests

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand/v2"
	"net/http"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vuathau/backend/internal/model"
)

// bigPhoto: ảnh cỡ thật như app chụp (1920x1440, JPEG 90, có nhiễu nên không nén nhỏ quá).
func bigPhoto(t *testing.T, seed uint64) []byte {
	t.Helper()
	rnd := rand.New(rand.NewPCG(seed, seed*13+5))
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1440))
	base := color.RGBA{uint8(rnd.IntN(256)), uint8(rnd.IntN(256)), uint8(rnd.IntN(256)), 255}
	for y := 0; y < 1440; y++ {
		for x := 0; x < 1920; x++ {
			n := uint8(rnd.IntN(40))
			img.SetRGBA(x, y, color.RGBA{base.R ^ uint8(x/60) + n, base.G ^ uint8(y/45) + n, base.B + n, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Thử tải nặng (chỉ chạy khi LOAD=1): n CTV cùng lúc mở app (home, danh sách hồ sơ), kiểm 1 ảnh vừa chụp và nộp
// hồ sơ 6 ảnh cỡ thật. In thời gian chờ p50 / p95 / chậm nhất từng loại và RAM lớn nhất của tiến trình.
//
//	LOAD=1 LOAD_N=100 go test ./tests/ -run TestHeavyLoad -v
func TestHeavyLoad(t *testing.T) {
	if os.Getenv("LOAD") == "" {
		t.Skip("đặt LOAD=1 để chạy thử tải nặng")
	}
	n := 100
	if v := os.Getenv("LOAD_N"); v != "" {
		_, _ = fmt.Sscan(v, &n)
	}
	e := newTestEnv(t, withRateLimit(10, 60))
	pics := make([][]byte, 12)
	for i := range pics {
		pics[i] = bigPhoto(t, 7000+uint64(i))
	}
	t.Logf("ảnh mẫu: %d KB", len(pics[0])/1024)
	ctvs := make([]authData, n)
	for i := range ctvs {
		ctvs[i] = e.register(t, fmt.Sprintf("CTV nặng %d", i))
	}

	var mu sync.Mutex
	lat := map[string][]time.Duration{}
	var fails atomic.Int32
	track := func(kind string, start time.Time, status, want int) {
		mu.Lock()
		lat[kind] = append(lat[kind], time.Since(start))
		mu.Unlock()
		if status != want {
			fails.Add(1)
			t.Errorf("%s: %d", kind, status)
		}
	}
	var peak atomic.Uint64
	stop := make(chan struct{})
	go func() {
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak.Load() {
					peak.Store(ms.HeapAlloc)
				}
			}
		}
	}()

	start := time.Now()
	var wg sync.WaitGroup
	for i, c := range ctvs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := time.Now()
			r := e.do(t, http.MethodGet, "/api/v1/ctv/home", c.AccessToken, nil)
			track("mở app (home)", s, r.Status, http.StatusOK)
			s = time.Now()
			r = e.do(t, http.MethodGet, "/api/v1/ctv/map", c.AccessToken, nil)
			track("bản đồ CTV", s, r.Status, http.StatusOK)
			files := make([][]byte, 6)
			for k := range files {
				files[k] = append([]byte(nil), pics[(i+k)%len(pics)]...)
				files[k] = append(files[k], byte(i), byte(k), byte(i>>8)) // khác file (không trùng sha) nhưng vẫn là JPEG hợp lệ
			}
			// như app: kiểm từng ảnh ngay lúc chụp, rồi nộp đúng các ảnh đó
			for _, f := range files {
				s = time.Now()
				r = e.upload(t, "/api/v1/photos/check", c.AccessToken, "file", [][]byte{f}, nil)
				track("kiểm 1 ảnh", s, r.Status, http.StatusOK)
			}
			s = time.Now()
			r = e.upload(t, "/api/v1/houses", c.AccessToken, "files", files, map[string]string{"meta": cameraMeta(6, 10.1+float64(i)/1000)})
			track("nộp 6 ảnh", s, r.Status, http.StatusCreated)
		}()
	}
	wg.Wait()
	close(stop)
	total := time.Since(start)
	for _, k := range []string{"mở app (home)", "bản đồ CTV", "kiểm 1 ảnh", "nộp 6 ảnh"} {
		d := lat[k]
		slices.Sort(d)
		if len(d) == 0 {
			continue
		}
		p := func(q float64) time.Duration { return d[min(len(d)-1, int(float64(len(d))*q))].Round(time.Millisecond) }
		t.Logf("%-16s n=%d  p50=%s  p95=%s  chậm nhất=%s", k, len(d), p(.5), p(.95), d[len(d)-1].Round(time.Millisecond))
	}
	t.Logf("%d CTV cùng lúc: xong trong %s, lỗi %d, RAM heap lớn nhất %d MB, CPU %d", n, total.Round(time.Millisecond), fails.Load(),
		peak.Load()>>20, runtime.NumCPU())
	_ = model.RoleCTV
}
