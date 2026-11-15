package handler

import (
	"net/http"

	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/service"
)

type UserHandler struct {
	svc service.UserService
}

func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Me handles GET /api/v1/users/me
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	uid, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	u, err := h.svc.GetByID(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusOK, u)
}

// GetByID handles GET /api/v1/users/{id}
func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		response.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}

	u, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	uid, _ := middleware.UserIDFromContext(r.Context())
	if u.ID != uid {
		u.Email = "" // emails are private: only visible via /users/me
	}
	response.Success(w, http.StatusOK, u)
}

// List handles GET /api/v1/users?limit=&offset=
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.List(r.Context(), queryInt(r, "limit", 0), queryInt(r, "offset", 0))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	uid, _ := middleware.UserIDFromContext(r.Context())
	for i := range users {
		if users[i].ID != uid {
			users[i].Email = ""
		}
	}
	response.Success(w, http.StatusOK, users)
}
