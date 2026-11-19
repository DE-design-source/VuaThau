package handler

import (
	"net/http"
	"time"

	"vuathau/backend/internal/integration/dezon"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/repository"
)

// DezonHandler: dữ liệu công khai trên dezon.vn — nhà thầu tiềm năng (trang Sale, đọc trang, đệm 6 giờ) và
// công trình nhà thầu mới đăng (lớp ghim riêng trên bản đồ, đồng bộ vào bảng dezon_projects).
type DezonHandler struct {
	c        *dezon.Client
	projects repository.DezonRepository // nil: chưa bật (test)
}

func NewDezonHandler(listURL string, projects repository.DezonRepository) *DezonHandler {
	return &DezonHandler{c: dezon.New(listURL, 6*time.Hour), projects: projects}
}

// Contractors handles GET /api/v1/sale/dezon-contractors
func (h *DezonHandler) Contractors(w http.ResponseWriter, r *http.Request) {
	l, err := h.c.Get(r.Context())
	if err != nil {
		response.Error(w, http.StatusBadGateway, "Chưa đọc được danh sách nhà thầu trên dezon.vn, thử lại sau ít phút.")
		return
	}
	ok(w, l)
}

// Projects handles GET /api/v1/dezon/projects?province=VN79 — công trình Dezon đã có toạ độ.
func (h *DezonHandler) Projects(w http.ResponseWriter, r *http.Request) {
	rows := []repository.DezonProject{}
	if h.projects != nil {
		var err error
		if rows, err = h.projects.Points(r.Context(), r.URL.Query().Get("province")); err != nil {
			response.Error(w, http.StatusInternalServerError, "Không đọc được công trình Dezon.")
			return
		}
	}
	ok(w, map[string]any{"rows": rows, "source": dezon.ProjectsURL})
}
