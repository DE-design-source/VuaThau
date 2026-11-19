package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"vuathau/backend/internal/pkg/response"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimit: giới hạn tốc độ trong bộ nhớ, mỗi "khách" 1 xô token.
// Khách = tài khoản đang đăng nhập (theo token), chưa đăng nhập thì theo IP thật của người dùng.
// (Trước đây theo địa chỉ kết nối: mọi người dùng đi qua Caddy trùng 1 IP nên chung 1 xô 10 yêu cầu/giây.)
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	var (
		mu       sync.Mutex
		visitors = make(map[string]*visitor)
	)

	// Evict idle visitors so the map doesn't grow forever.
	go func() {
		for range time.Tick(time.Minute) {
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientKey(r)

			mu.Lock()
			v, ok := visitors[ip]
			if !ok {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[ip] = v
			}
			v.lastSeen = time.Now()
			allowed := v.limiter.Allow()
			mu.Unlock()

			if !allowed {
				w.Header().Set("Retry-After", "1")
				response.Error(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientKey: khoá xô giới hạn: token đăng nhập (băm) nếu có, không thì IP thật.
func clientKey(r *http.Request) string {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") && len(a) > 20 {
		h := sha256.Sum256([]byte(a))
		return "t:" + hex.EncodeToString(h[:12])
	}
	return "ip:" + ClientIP(r)
}

// ClientIP: IP thật của người dùng. Kết nối đến từ mạng nội bộ (Caddy trong docker) thì lấy địa chỉ cuối cùng
// trong X-Forwarded-For (Caddy tự thêm, người dùng không giả được); kết nối thẳng thì lấy địa chỉ kết nối.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
				return last
			}
		}
	}
	return host
}

// Slow: ghi nhật ký yêu cầu chạy lâu hơn limit (đo chỗ nghẽn thật), không ghi query string (có thể chứa chữ ký link).
func Slow(limit time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(sw, r)
			if d := time.Since(start); d > limit {
				log.Printf("chậm %s %s %d %s (%d byte vào)", r.Method, r.URL.Path, sw.code, d.Round(time.Millisecond), r.ContentLength)
			}
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) {
	w.code = c
	w.ResponseWriter.WriteHeader(c)
}

// Flush: giữ khả năng đẩy dữ liệu dần (tải video, file lớn).
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap: http.ResponseController (ServeContent, đặt hạn ghi) tìm được ResponseWriter gốc.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
