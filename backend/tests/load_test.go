package tests

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Nhiều CTV cùng dùng app (cùng 1 IP như khi đi qua Caddy): mỗi người mở app vài lần và kiểm 1 ảnh vừa chụp.
// Không ai bị 429 (trước đây mọi người chung 1 xô giới hạn theo IP của Caddy), mọi yêu cầu thành công.
func TestManyCTVsAtOnce(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t, withRateLimit(10, 40)) // giống cấu hình chạy thật
	const n = 30
	ctvs := make([]authData, n)
	for i := range ctvs {
		ctvs[i] = e.register(t, fmt.Sprintf("CTV tải %d", i))
	}
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan string, n*10)
	for i, c := range ctvs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 6 {
				if r := e.do(t, http.MethodGet, "/api/v1/ctv/home", c.AccessToken, nil); r.Status != http.StatusOK {
					errs <- fmt.Sprintf("home CTV %d: %d", i, r.Status)
				}
			}
			r := e.upload(t, "/api/v1/photos/check", c.AccessToken, "file", [][]byte{photo(t, 9000+uint64(i))}, map[string]string{"meta": cameraMeta(1, 10.8)[1 : len(cameraMeta(1, 10.8))-1]})
			if r.Status != http.StatusOK {
				errs <- fmt.Sprintf("check CTV %d: %d %s", i, r.Status, r.Raw)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for m := range errs {
		t.Error(m)
	}
	t.Logf("%d CTV x 7 yêu cầu: %s", n, time.Since(start).Round(time.Millisecond))
}
