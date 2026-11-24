package handler

import (
	"net/http"

	"vuathau/backend/internal/service"
)

// FeedbackHandler: đánh giá app (CTV / nhà thầu gửi, admin xem).
type FeedbackHandler struct{ svc *service.FeedbackService }

func NewFeedbackHandler(svc *service.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{svc: svc}
}

// Ask handles GET /api/v1/feedback/ask
func (h *FeedbackHandler) Ask(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Ask(r.Context(), me(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Submit handles POST /api/v1/feedback
func (h *FeedbackHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var in service.FeedbackInput
	if !bodyOr400(w, r, &in) {
		return
	}
	msg, err := h.svc.Submit(r.Context(), me(r), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]string{"message": msg})
}

// List handles GET /api/v1/admin/feedback
func (h *FeedbackHandler) List(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}
