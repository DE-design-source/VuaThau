package handler

import (
	"fmt"
	"net/http"

	"vuathau/backend/internal/service"
)

// ContractorHandler: trang Nhà thầu của admin (đăng ký qua form Lark + trích từ ảnh CTV).
type ContractorHandler struct {
	svc     *service.ContractorService
	formURL string // form Lark đăng ký nhà thầu (nút "Thêm nhà thầu")
}

func NewContractorHandler(svc *service.ContractorService, formURL string) *ContractorHandler {
	return &ContractorHandler{svc: svc, formURL: formURL}
}

// List handles GET /api/v1/contractors -> { rows, form_url }
func (h *ContractorHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows, "form_url": h.formURL})
}

// Get handles GET /api/v1/contractors/{key}
func (h *ContractorHandler) Get(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Get(r.Context(), r.PathValue("key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, d)
}

// Sync handles POST /api/v1/contractors/sync: đọc lại form Lark ngay.
func (h *ContractorHandler) Sync(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.SyncLark(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"synced": n, "message": fmt.Sprintf("Đã đọc %d nhà thầu từ form đăng ký.", n)})
}
