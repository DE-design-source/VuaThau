package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	req := func(remote, xff string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	for _, c := range []struct{ remote, xff, want string }{
		{"172.20.0.5:4000", "113.161.1.2", "113.161.1.2"},           // qua Caddy trong docker
		{"172.20.0.5:4000", "1.1.1.1, 113.161.1.2", "113.161.1.2"}, // người dùng tự ghi XFF giả: lấy địa chỉ Caddy thêm (cuối)
		{"203.0.113.9:4000", "1.1.1.1", "203.0.113.9"},             // kết nối thẳng từ Internet: không tin XFF
		{"172.20.0.5:4000", "", "172.20.0.5"},
		{"127.0.0.1:4000", "rác", "127.0.0.1"},
	} {
		if got := ClientIP(req(c.remote, c.xff)); got != c.want {
			t.Errorf("ClientIP(%s, %q) = %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

// Qua Caddy mọi người cùng 1 địa chỉ kết nối: mỗi tài khoản / mỗi IP thật phải có xô riêng.
func TestRateLimitPerClient(t *testing.T) {
	h := RateLimit(1, 2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	hit := func(token, ip string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "172.20.0.5:4000"
		r.Header.Set("X-Forwarded-For", ip)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for range 2 {
		if c := hit("token-cua-ctv-a-0123456789", "1.2.3.4"); c != http.StatusOK {
			t.Fatalf("A: %d", c)
		}
	}
	if c := hit("token-cua-ctv-a-0123456789", "1.2.3.4"); c != http.StatusTooManyRequests {
		t.Fatalf("A lần 3: %d", c)
	}
	// CTV B cùng mạng (cùng IP) vẫn dùng được; người chưa đăng nhập ở IP khác cũng vậy
	if c := hit("token-cua-ctv-b-0123456789", "1.2.3.4"); c != http.StatusOK {
		t.Fatalf("B: %d", c)
	}
	if c := hit("", "5.6.7.8"); c != http.StatusOK {
		t.Fatalf("IP khác: %d", c)
	}
}
