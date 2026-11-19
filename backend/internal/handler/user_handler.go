package handler

import (
	"net/http"

	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/service"
)

type UserHandler struct {
	svc  service.UserService
	auth service.AuthService
}

func NewUserHandler(svc service.UserService, auth service.AuthService) *UserHandler {
	return &UserHandler{svc: svc, auth: auth}
}

// Me handles GET /api/v1/me (và /api/v1/users/me)
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Me(r.Context(), middleware.UserFromContext(r.Context()))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// UpdateBank handles PUT /api/v1/me/bank
func (h *UserHandler) UpdateBank(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"bank_name"`
		No    string `json:"bank_no"`
		Owner string `json:"bank_owner"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	u, err := h.svc.UpdateBank(r.Context(), middleware.UserFromContext(r.Context()), in.Name, in.No, in.Owner)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": "Đã cập nhật tài khoản nhận tiền.", "user": u})
}

// ChangePassword handles PUT /api/v1/me/password: trả token mới (token cũ hết hiệu lực).
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	u, err := h.svc.ChangePassword(r.Context(), middleware.UserFromContext(r.Context()), in.Old, in.New)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	res, err := h.auth.Issue(u)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": "Đã đổi mật khẩu.", "access_token": res.AccessToken, "expires_in": res.ExpiresIn})
}

// DeleteSelf handles DELETE /api/v1/me (CTV tự xoá tài khoản)
func (h *UserHandler) DeleteSelf(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.DeleteSelf(r.Context(), middleware.UserFromContext(r.Context()))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// Staff handles GET /api/v1/staff
func (h *UserHandler) Staff(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListStaff(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows})
}

// AddStaff handles POST /api/v1/staff
func (h *UserHandler) AddStaff(w http.ResponseWriter, r *http.Request) {
	var in service.AddStaffInput
	if !bodyOr400(w, r, &in) {
		return
	}
	u, m, err := h.svc.AddStaff(r.Context(), middleware.UserFromContext(r.Context()), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, map[string]any{"message": m, "user": u})
}

// AddCTV handles POST /api/v1/ctvs {name, phone, password, bank_name?, bank_no?, bank_owner?}
func (h *UserHandler) AddCTV(w http.ResponseWriter, r *http.Request) {
	var in service.AddCTVInput
	if !bodyOr400(w, r, &in) {
		return
	}
	u, m, err := h.svc.AddCTV(r.Context(), middleware.UserFromContext(r.Context()), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, map[string]any{"message": m, "user": u})
}

// Lock handles POST /api/v1/users/{id}/lock {locked}
func (h *UserHandler) Lock(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in struct {
		Locked bool `json:"locked"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.SetLocked(r.Context(), middleware.UserFromContext(r.Context()), id, in.Locked)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// ResetPassword handles POST /api/v1/users/{id}/password {password}
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.ResetPassword(r.Context(), middleware.UserFromContext(r.Context()), id, in.Password)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// CTVs handles GET /api/v1/ctvs?q=&page=
func (h *UserHandler) CTVs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.CTVFilter{Q: q.Get("q"), Region: q.Get("region"), Kind: q.Get("kind"), Bank: q.Get("bank")}
	rows, found, page, err := h.svc.ListCTVs(r.Context(), f, queryInt(r, "page", 1))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	facets, err := h.svc.CTVFacets(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows, "found": found, "page": page, "pages": max(1, (found+service.PageSize-1)/service.PageSize),
		"facets": facets})
}

// CTVProfile handles PUT /api/v1/ctvs/{id}/profile (admin): khu vực, chức năng, ghi chú.
func (h *UserHandler) CTVProfile(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in service.CTVProfileInput
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.SetCTVProfile(r.Context(), me(r), id, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": m})
}

// CTVKinds handles PUT /api/v1/ctv-kinds (admin): {kinds: [...]}
func (h *UserHandler) CTVKinds(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kinds []string `json:"kinds"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	kinds, m, err := h.svc.SaveKinds(r.Context(), me(r), in.Kinds)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": m, "kinds": kinds})
}

// CTV handles GET /api/v1/ctvs/{id}
func (h *UserHandler) CTV(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	d, err := h.svc.CTV(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, d)
}
