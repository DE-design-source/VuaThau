package handler

import (
	"net/http"

	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/service"
)

type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var in service.RegisterInput
	if err := decodeJSON(w, r, &in); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	res, err := h.svc.Register(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, res)
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in service.LoginInput
	if err := decodeJSON(w, r, &in); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	res, err := h.svc.Login(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusOK, res)
}
