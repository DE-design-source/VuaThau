package handler

import (
	"net/http"
	"time"

	"vuathau/backend/internal/integration/dezon"
	"vuathau/backend/internal/pkg/response"
)

// DezonHandler: nhà thầu tiềm năng trên dezon.vn cho trang Sale (đọc trang công khai, đệm 6 giờ).
type DezonHandler struct{ c *dezon.Client }

func NewDezonHandler(listURL string) *DezonHandler {
	return &DezonHandler{c: dezon.New(listURL, 6*time.Hour)}
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
