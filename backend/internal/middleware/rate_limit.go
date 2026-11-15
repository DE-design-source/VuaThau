package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"vuathau/backend/internal/pkg/response"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimit applies an in-memory token bucket per client IP.
// For multiple instances behind a load balancer, switch to a Redis-backed limiter.
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
			ip := clientIP(r)

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

// clientIP uses the TCP peer address. If running behind a trusted reverse
// proxy, read X-Forwarded-For / X-Real-IP here instead.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
