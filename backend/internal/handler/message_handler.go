package handler

import (
	"net/http"

	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/service"
)

type MessageHandler struct {
	svc service.MessageService
}

func NewMessageHandler(svc service.MessageService) *MessageHandler {
	return &MessageHandler{svc: svc}
}

// Send handles POST /api/v1/messages
func (h *MessageHandler) Send(w http.ResponseWriter, r *http.Request) {
	uid, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var in service.SendMessageInput
	if err := decodeJSON(w, r, &in); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	msg, err := h.svc.Send(r.Context(), uid, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, msg)
}

// Conversation handles GET /api/v1/messages/{userID}?limit=&offset=
func (h *MessageHandler) Conversation(w http.ResponseWriter, r *http.Request) {
	uid, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	otherID, ok := pathID(r, "userID")
	if !ok {
		response.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}

	msgs, err := h.svc.ListConversation(r.Context(), uid, otherID, queryInt(r, "limit", 0), queryInt(r, "offset", 0))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusOK, msgs)
}
