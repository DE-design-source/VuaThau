package handler

import (
	"fmt"
	"net/http"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/service"
)

type PaymentHandler struct {
	svc            service.PaymentService
	googleClientID string
}

func NewPaymentHandler(svc service.PaymentService, googleClientID string) *PaymentHandler {
	return &PaymentHandler{svc: svc, googleClientID: googleClientID}
}

func sendFile(w http.ResponseWriter, data []byte, mime, filename string) {
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write(data)
}

// Config handles GET /api/v1/config (công khai)
func (h *PaymentHandler) Config(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Config(r.Context(), h.googleClientID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, c)
}

// Settings handles GET /api/v1/settings
func (h *PaymentHandler) Settings(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Settings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, s)
}

// SaveSettings handles PUT /api/v1/settings
func (h *PaymentHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var in model.Settings
	if !bodyOr400(w, r, &in) {
		return
	}
	m, s, err := h.svc.SaveSettings(r.Context(), me(r), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": m, "settings": s})
}

// Accounting handles GET /api/v1/accounting
func (h *PaymentHandler) Accounting(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Accounting(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Export handles GET /api/v1/accounting/export?format=xlsx|csv&ids=
func (h *PaymentHandler) Export(w http.ResponseWriter, r *http.Request) {
	data, mime, name, err := h.svc.Export(r.Context(), r.URL.Query().Get("format"), idsOf(r, "ids"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	sendFile(w, data, mime, name)
}

// PayMany handles POST /api/v1/accounting/pay {ref, items:[{ctv_id,total}]}
func (h *PaymentHandler) PayMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ref   string            `json:"ref"`
		Items []service.PayItem `json:"items"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	res, err := h.svc.PayMany(r.Context(), me(r), in.Ref, in.Items)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Payout handles GET /api/v1/payouts/{ctv_id}
func (h *PaymentHandler) Payout(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "ctv_id")
	if !valid {
		return
	}
	res, err := h.svc.Payout(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Pay handles POST /api/v1/payouts/{ctv_id}
func (h *PaymentHandler) Pay(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "ctv_id")
	if !valid {
		return
	}
	var in service.PayInput
	if !bodyOr400(w, r, &in) {
		return
	}
	m, bid, err := h.svc.Pay(r.Context(), me(r), id, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, map[string]any{"message": m, "batch_id": bid})
}

// Batches handles GET /api/v1/batches
func (h *PaymentHandler) Batches(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Batches(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Batch handles GET /api/v1/batches/{id} (nhân viên, và CTV của phiếu)
func (h *PaymentHandler) Batch(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	res, err := h.svc.Batch(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// SetRef handles PUT /api/v1/batches/{id} {ref}
func (h *PaymentHandler) SetRef(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in struct {
		Ref string `json:"ref"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.SetRef(r.Context(), me(r), id, in.Ref)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// Cancel handles POST /api/v1/batches/{id}/cancel {reason}
func (h *PaymentHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.Cancel(r.Context(), me(r), id, in.Reason)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// UploadReceipt handles POST /api/v1/batches/{id}/receipt (multipart file)
func (h *PaymentHandler) UploadReceipt(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	files, valid := readFiles(w, r, "file")
	if !valid {
		return
	}
	if len(files) == 0 {
		response.Error(w, http.StatusBadRequest, "Chưa chọn ảnh biên lai.")
		return
	}
	m, err := h.svc.UploadReceipt(r.Context(), me(r), id, files[0].Data)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// Receipt handles GET /api/v1/batches/{id}/receipt
func (h *PaymentHandler) Receipt(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	data, err := h.svc.Receipt(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

// AddAdjustment handles POST /api/v1/ctvs/{id}/adjustments {amount, reason}
func (h *PaymentHandler) AddAdjustment(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.AddAdjustment(r.Context(), me(r), id, in.Amount, in.Reason)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, map[string]any{"message": m})
}

// DeleteAdjustment handles DELETE /api/v1/adjustments/{id}
func (h *PaymentHandler) DeleteAdjustment(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	m, err := h.svc.DeleteAdjustment(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// Report handles GET /api/v1/reports?from=&to=&group=
func (h *PaymentHandler) Report(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.Report(r.Context(), q.Get("from"), q.Get("to"), q.Get("group"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// ReportExport handles GET /api/v1/reports/export?from=&to=
func (h *PaymentHandler) ReportExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, name, err := h.svc.ReportExport(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	sendFile(w, data, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", name)
}
