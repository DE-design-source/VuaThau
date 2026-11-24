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

// Register handles POST /api/v1/auth/register (CTV đăng ký trên app)
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var in service.RegisterInput
	if !bodyOr400(w, r, &in) {
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
	if !bodyOr400(w, r, &in) {
		return
	}
	res, err := h.svc.Login(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Phone handles POST /api/v1/auth/phone {phone, as}: CTV / nhà thầu vào bằng SĐT.
// as = "ctv" | "nhathau" (nút chọn ở màn đăng nhập); trống: app cũ, không kiểm vai trò.
func (h *AuthHandler) Phone(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
		As    string `json:"as"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	res, err := h.svc.PhoneLogin(r.Context(), in.Phone, in.As)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Google handles POST /api/v1/auth/google {id_token}
func (h *AuthHandler) Google(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDToken string `json:"id_token"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	res, err := h.svc.Google(r.Context(), in.IDToken)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}
