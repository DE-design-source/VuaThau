package tests

import (
	"net/http"
	"strings"
	"testing"

	"vuathau/backend/internal/model"
)

// Quy tắc duyệt ảnh cho AI (góp ý 21/11/2026): admin sửa ở trang Cấu hình, AI nhận kèm từ lượt đọc ảnh tiếp theo.
func TestAIRules(t *testing.T) {
	t.Parallel()
	fa := &fakeAI{}
	e := newTestEnv(t, func(s *envSettings) { s.ai = fa })
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV quy tắc")

	var g struct {
		Rules         string `json:"rules"`
		DefaultPrompt string `json:"default_prompt"`
		Max           int    `json:"max"`
	}
	r := e.do(t, http.MethodGet, "/api/v1/settings/ai-rules", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &g)
	if g.Rules != "" || !strings.Contains(g.DefaultPrompt, "stage") || g.Max < 1000 {
		t.Fatalf("quy tắc ban đầu: %s", r.Raw)
	}
	// CTV không xem / sửa được
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/settings/ai-rules", ctv.AccessToken, nil), http.StatusForbidden)
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings/ai-rules", ctv.AccessToken, map[string]string{"rules": "x"}), http.StatusForbidden)
	// dài quá: không lưu
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings/ai-rules", admin.AccessToken, map[string]string{"rules": strings.Repeat("a", g.Max+1)}),
		http.StatusBadRequest)

	rules := "Ảnh chụp bảng giấy phép xây dựng cũng tính là ảnh công trình (is_site = true)."
	r = e.do(t, http.MethodPut, "/api/v1/settings/ai-rules", admin.AccessToken, map[string]string{"rules": "  " + rules + "\r\n"})
	expectStatus(t, r, http.StatusOK)
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/settings/ai-rules", admin.AccessToken, nil), &g)
	if g.Rules != rules {
		t.Fatalf("quy tắc đã lưu: %q", g.Rules)
	}

	// AI đọc ảnh nhận kèm quy tắc
	up := e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{bigPhoto(t, 9900)}, nil)
	expectStatus(t, up, http.StatusOK)
	fa.mu.Lock()
	got := fa.rules
	fa.mu.Unlock()
	if got != rules {
		t.Fatalf("AI nhận quy tắc %q, cần %q", got, rules)
	}

	// xoá quy tắc: AI về lời dặn mặc định
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings/ai-rules", admin.AccessToken, map[string]string{"rules": ""}), http.StatusOK)
	expectStatus(t, e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{bigPhoto(t, 9901)}, nil), http.StatusOK)
	fa.mu.Lock()
	got = fa.rules
	fa.mu.Unlock()
	if got != "" {
		t.Fatalf("xoá quy tắc mà AI vẫn nhận: %q", got)
	}
}
