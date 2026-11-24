package handler

import (
	"mime/multipart"
	"net/http"
	"os"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/service"
)

// NTHandler: API cho tài khoản nhà thầu trên app (/api/v1/nt/...) và trang admin nhà thầu.
type NTHandler struct{ svc *service.NTService }

func NewNTHandler(svc *service.NTService) *NTHandler { return &NTHandler{svc: svc} }

// Home handles GET /api/v1/nt/home
func (h *NTHandler) Home(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Home(r.Context(), me(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// SaveProfile handles PUT /api/v1/nt/profile
func (h *NTHandler) SaveProfile(w http.ResponseWriter, r *http.Request) {
	var in service.NTProfileInput
	if !bodyOr400(w, r, &in) {
		return
	}
	res, err := h.svc.SaveProfile(r.Context(), me(r), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": "Đã lưu hồ sơ công ty.", "profile": res})
}

// Projects handles GET /api/v1/nt/projects?status=
func (h *NTHandler) Projects(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Projects(r.Context(), me(r), r.URL.Query().Get("status"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows})
}

// CreateProject handles POST /api/v1/nt/projects
func (h *NTHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var in service.NTProjectInput
	if !bodyOr400(w, r, &in) {
		return
	}
	p, err := h.svc.CreateProject(r.Context(), me(r), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, map[string]any{"message": "Đã tạo dự án " + p.Name + ".", "project": p})
}

// Project handles GET /api/v1/nt/projects/{id}
func (h *NTHandler) Project(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	res, err := h.svc.Project(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// UpdateProject handles PUT /api/v1/nt/projects/{id}
func (h *NTHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in service.NTProjectInput
	if !bodyOr400(w, r, &in) {
		return
	}
	p, err := h.svc.UpdateProject(r.Context(), me(r), id, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": "Đã lưu dự án.", "project": p})
}

// DeleteProject handles DELETE /api/v1/nt/projects/{id}
func (h *NTHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	m, err := h.svc.DeleteProject(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// AddLog handles POST /api/v1/nt/projects/{id}/logs (multipart: note, meta, files): đọc dần, không giữ video trong RAM.
func (h *NTHandler) AddLog(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(model.MaxPhotos)*(service.MaxFileBytes+1<<20)+int64(model.MaxVideos)*(service.MaxVideoBytes+1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Không đọc được ảnh / video gửi lên.")
		return
	}
	var part *multipart.Part
	next := func() (*service.NTPart, error) {
		if part != nil {
			_ = part.Close()
		}
		p, err := mr.NextPart()
		if err != nil {
			return nil, err // io.EOF: hết
		}
		part = p
		return &service.NTPart{Field: p.FormName(), Name: p.FileName(), Mime: p.Header.Get("Content-Type"), Body: p}, nil
	}
	res, err := h.svc.AddLog(r.Context(), me(r), id, next)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, res)
}

// Sites handles GET /api/v1/nt/sites: công trình CTV đã chụp có SĐT nhà thầu này ({rows})
func (h *NTHandler) Sites(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Sites(r.Context(), me(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows})
}

// AddProjectPhotos handles POST /api/v1/nt/projects/{id}/photos (multipart files): ảnh dự án, ảnh đầu = ảnh bìa.
func (h *NTHandler) AddProjectPhotos(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(service.MaxNTProjectPhotos)*(service.MaxFileBytes+1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Không đọc được ảnh gửi lên.")
		return
	}
	var part *multipart.Part
	next := func() (*service.NTPart, error) {
		if part != nil {
			_ = part.Close()
		}
		p, err := mr.NextPart()
		if err != nil {
			return nil, err
		}
		part = p
		return &service.NTPart{Field: p.FormName(), Name: p.FileName(), Mime: p.Header.Get("Content-Type"), Body: p}, nil
	}
	res, err := h.svc.AddProjectPhotos(r.Context(), me(r), id, next)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, res)
}

// Report handles GET /api/v1/nt/projects/{id}/logs/{lid}/report: link báo cáo gửi chủ nhà
func (h *NTHandler) Report(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	lid, valid := idOr400(w, r, "lid")
	if !valid {
		return
	}
	res, err := h.svc.Report(r.Context(), me(r), id, lid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// PublicReport handles GET /api/v1/public/bc/{code} (không cần đăng nhập)
func (h *NTHandler) PublicReport(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.PublicReport(r.Context(), r.PathValue("code"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	ok(w, res)
}

// PublicReportPhoto handles GET /api/v1/public/bc/{code}/photos/{mid}?size=small
func (h *NTHandler) PublicReportPhoto(w http.ResponseWriter, r *http.Request) {
	mid, valid := idOr400(w, r, "mid")
	if !valid {
		return
	}
	b, err := h.svc.PublicReportPhoto(r.Context(), r.PathValue("code"), mid, r.URL.Query().Get("size") == "small")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

// Divisions handles GET /api/v1/geo/divisions: 34 tỉnh / thành + phường / xã (form Đăng dự án)
func Divisions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(province.WardsJSON)
}

// DeleteLog handles DELETE /api/v1/nt/projects/{id}/logs/{lid}
func (h *NTHandler) DeleteLog(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	lid, valid := idOr400(w, r, "lid")
	if !valid {
		return
	}
	m, err := h.svc.DeleteLog(r.Context(), me(r), id, lid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// Photo handles GET /api/v1/nt/photos/{id}?size=small
func (h *NTHandler) Photo(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	b, err := h.svc.Photo(r.Context(), me(r), id, r.URL.Query().Get("size") == "small")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(b)
}

// Video handles GET /api/v1/nt/videos/{id}?e=&s= (link có chữ ký, không cần token)
func (h *NTHandler) Video(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	p, mime, err := h.svc.VideoFile(r.Context(), id, r.URL.Query().Get("e"), r.URL.Query().Get("s"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		response.Error(w, http.StatusNotFound, "Không tìm thấy file video.")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		response.Error(w, http.StatusNotFound, "Không tìm thấy file video.")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=43200")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// Contractors handles GET /api/v1/admin/nt (admin): danh sách nhà thầu dùng app
func (h *NTHandler) Contractors(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Contractors(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows})
}

// AddContractor handles POST /api/v1/admin/nt (admin): {phone, name, company}
func (h *NTHandler) AddContractor(w http.ResponseWriter, r *http.Request) {
	var in service.NTAddInput
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.AddContractor(r.Context(), me(r), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// AdminProject handles GET /api/v1/admin/nt/projects/{id} (admin xem dự án của nhà thầu)
func (h *NTHandler) AdminProject(w http.ResponseWriter, r *http.Request) { h.Project(w, r) }
