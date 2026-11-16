package handler

import (
	"io"
	"net/http"
	"strconv"

	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/service"
)

type HouseHandler struct {
	svc service.HouseService
}

func NewHouseHandler(svc service.HouseService) *HouseHandler {
	return &HouseHandler{svc: svc}
}

const maxUploadBytes = (model.MaxPhotos + 1) * (service.MaxFileBytes + 1<<20)

// readFiles đọc các file của trường key trong form multipart (tối đa 20 ảnh x 20MB).
func readFiles(w http.ResponseWriter, r *http.Request, key string) ([]service.Upload, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "Không đọc được ảnh gửi lên (tối đa 20 ảnh, mỗi ảnh dưới 20MB).")
		return nil, false
	}
	out := []service.Upload{}
	for _, fh := range r.MultipartForm.File[key] {
		f, err := fh.Open()
		if err != nil {
			response.Error(w, http.StatusBadRequest, "Không đọc được ảnh gửi lên.")
			return nil, false
		}
		data, err := io.ReadAll(io.LimitReader(f, service.MaxFileBytes+1))
		_ = f.Close()
		if err != nil {
			response.Error(w, http.StatusBadRequest, "Không đọc được ảnh gửi lên.")
			return nil, false
		}
		out = append(out, service.Upload{Name: fh.Filename, Data: data})
	}
	return out, true
}

func me(r *http.Request) *model.User { return middleware.UserFromContext(r.Context()) }

// Home handles GET /api/v1/ctv/home?tab=
func (h *HouseHandler) Home(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.CTVHome(r.Context(), me(r), r.URL.Query().Get("tab"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Income handles GET /api/v1/ctv/income
func (h *HouseHandler) Income(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Income(r.Context(), me(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Geocode handles GET /api/v1/geocode?lat=&lng=
func (h *HouseHandler) Geocode(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		response.Error(w, http.StatusBadRequest, "Toạ độ không hợp lệ.")
		return
	}
	ok(w, map[string]string{"address": h.svc.Geocode(r.Context(), lat, lng)})
}

// CheckPhoto handles POST /api/v1/photos/check (multipart file, meta)
func (h *HouseHandler) CheckPhoto(w http.ResponseWriter, r *http.Request) {
	files, valid := readFiles(w, r, "file")
	if !valid {
		return
	}
	if len(files) == 0 {
		response.Error(w, http.StatusBadRequest, "Chưa có ảnh.")
		return
	}
	res, err := h.svc.CheckPhoto(r.Context(), me(r), files[0], r.FormValue("meta"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Create handles POST /api/v1/houses (multipart files, meta)
func (h *HouseHandler) Create(w http.ResponseWriter, r *http.Request) {
	files, valid := readFiles(w, r, "files")
	if !valid {
		return
	}
	res, err := h.svc.Create(r.Context(), me(r), files, r.FormValue("meta"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	status := http.StatusOK
	if res.HouseID != nil {
		status = http.StatusCreated
	}
	response.Success(w, status, res)
}

// AddPhotos handles POST /api/v1/houses/{id}/photos
func (h *HouseHandler) AddPhotos(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	files, valid := readFiles(w, r, "files")
	if !valid {
		return
	}
	res, err := h.svc.AddPhotos(r.Context(), me(r), id, files, r.FormValue("meta"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// DeletePhoto handles DELETE /api/v1/houses/{id}/photos/{pid}
func (h *HouseHandler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	pid, valid := idOr400(w, r, "pid")
	if !valid {
		return
	}
	m, err := h.svc.DeletePhoto(r.Context(), me(r), id, pid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

type saveInput struct {
	model.HouseFields
	Submit bool `json:"submit"`
}

// Save handles PUT /api/v1/houses/{id}: CTV lưu nháp hoặc gửi duyệt.
func (h *HouseHandler) Save(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in saveInput
	if !bodyOr400(w, r, &in) {
		return
	}
	m, submitted, err := h.svc.Save(r.Context(), me(r), id, in.HouseFields, in.Submit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": m, "submitted": submitted})
}

// State handles GET /api/v1/houses/{id}/state
func (h *HouseHandler) State(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	res, err := h.svc.State(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Detail handles GET /api/v1/houses/{id}
func (h *HouseHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	res, err := h.svc.Detail(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Photo handles GET /api/v1/photos/{pid}[?original=1]: trả file ảnh.
func (h *HouseHandler) Photo(w http.ResponseWriter, r *http.Request) {
	pid, valid := idOr400(w, r, "pid")
	if !valid {
		return
	}
	data, mime, err := h.svc.PhotoFile(r.Context(), me(r), pid, r.URL.Query().Get("original") == "1")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

// Queue handles GET /api/v1/admin/queue
func (h *HouseHandler) Queue(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Queue(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"rows": rows})
}

// ApproveMany handles POST /api/v1/admin/approve {ids}
func (h *HouseHandler) ApproveMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	m, n, err := h.svc.ApproveMany(r.Context(), me(r), in.IDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, map[string]any{"message": m, "approved": n})
}

// Review handles POST /api/v1/houses/{id}/review {action, reason}
func (h *HouseHandler) Review(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if !bodyOr400(w, r, &in) {
		return
	}
	res, err := h.svc.Review(r.Context(), me(r), id, in.Action, in.Reason)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Undo handles POST /api/v1/houses/{id}/undo
func (h *HouseHandler) Undo(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	m, err := h.svc.Undo(r.Context(), me(r), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// EditInfo handles PUT /api/v1/houses/{id}/info (admin)
func (h *HouseHandler) EditInfo(w http.ResponseWriter, r *http.Request) {
	id, valid := idOr400(w, r, "id")
	if !valid {
		return
	}
	var in model.HouseFields
	if !bodyOr400(w, r, &in) {
		return
	}
	m, err := h.svc.EditInfo(r.Context(), me(r), id, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// SetAmount handles PUT /api/v1/houses/{id}/amount {amount, reason}
func (h *HouseHandler) SetAmount(w http.ResponseWriter, r *http.Request) {
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
	m, err := h.svc.SetAmount(r.Context(), me(r), id, in.Amount, in.Reason)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	msg(w, m)
}

// Overview handles GET /api/v1/houses?status=&q=&ctv=&from=&to=&page=
func (h *HouseHandler) Overview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctv, _ := strconv.ParseInt(q.Get("ctv"), 10, 64)
	res, err := h.svc.Overview(r.Context(), service.OverviewQuery{Status: q.Get("status"), Q: q.Get("q"), CTV: ctv,
		From: q.Get("from"), To: q.Get("to"), Province: q.Get("province"), Page: queryInt(r, "page", 1)})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// Map handles GET /api/v1/map?ctv=&status=&from=&to=
func (h *HouseHandler) Map(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctv, _ := strconv.ParseInt(q.Get("ctv"), 10, 64)
	res, err := h.svc.Map(r.Context(), service.OverviewQuery{Status: q.Get("status"), CTV: ctv, From: q.Get("from"), To: q.Get("to"),
		Province: q.Get("province")})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// InboxStatus handles GET /api/v1/admin/inbox
func (h *HouseHandler) InboxStatus(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.InboxStatus(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}

// InboxSync handles POST /api/v1/admin/inbox/sync: đồng bộ hồ sơ Messenger ngay.
func (h *HouseHandler) InboxSync(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.SyncInbox(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	ok(w, res)
}
// Provinces handles GET /api/v1/geo/provinces: ranh giới 34 tỉnh, thành (GeoJSON, công khai, lưu đệm lâu).
func (h *HouseHandler) Provinces(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(province.GeoJSON)
}