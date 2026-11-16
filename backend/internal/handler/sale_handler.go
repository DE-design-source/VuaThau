package handler

import (
	"net/http"
	"strconv"

	"vuathau/backend/internal/repository"
)

// Trang sale: bản đồ, dự án, nhà thầu (không có CTV, không có tiền).

// SaleMap handles GET /api/v1/sale/map?province=&ctv=
func (h *HouseHandler) SaleMap(w http.ResponseWriter, r *http.Request) {
	ctv, _ := strconv.ParseInt(r.URL.Query().Get("ctv"), 10, 64)
	res, err := h.svc.SaleMap(r.Context(), r.URL.Query().Get("province"), ctv)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// SaleProjects handles GET /api/v1/sale/projects?q=&province=&contractor=&page=
func (h *HouseHandler) SaleProjects(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.SaleProjects(r.Context(), repository.SaleFilter{Q: q.Get("q"), Province: q.Get("province"),
		Contractor: q.Get("contractor")}, queryInt(r, "page", 1))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// SaleProject handles GET /api/v1/sale/projects/{id}
func (h *HouseHandler) SaleProject(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	res, err := h.svc.SaleProject(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Contractors handles GET /api/v1/sale/contractors?q=
func (h *HouseHandler) Contractors(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Contractors(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": res})
}
