package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/service"
)

const maxBodyBytes = 1 << 20 // 1 MB

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	return json.NewDecoder(r.Body).Decode(dst)
}

// writeServiceError maps service errors to HTTP responses.
func writeServiceError(w http.ResponseWriter, err error) {
	msg := service.PublicMessage(err)
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, msg)
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrUnauthorized):
		response.Error(w, http.StatusUnauthorized, msg)
	case errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrLocked):
		response.Error(w, http.StatusForbidden, msg)
	case errors.Is(err, service.ErrConflict):
		response.Error(w, http.StatusConflict, msg)
	case errors.Is(err, service.ErrNotFound):
		response.Error(w, http.StatusNotFound, msg)
	case errors.Is(err, service.ErrUnavailable):
		response.Error(w, http.StatusServiceUnavailable, msg)
	case errors.Is(err, service.ErrTooManyAttempts):
		response.Error(w, http.StatusTooManyRequests, msg)
	default:
		log.Printf("internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "Máy chủ đang lỗi, thử lại sau ít phút.")
	}
}

func queryInt(r *http.Request, key string, fallback int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return fallback
}

func pathID(r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	return id, err == nil && id > 0
}

// idOr400 đọc {key} trên đường dẫn; sai thì trả 400 và ok=false.
func idOr400(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, ok := pathID(r, key)
	if !ok {
		response.Error(w, http.StatusBadRequest, "Mã không hợp lệ.")
	}
	return id, ok
}

// uid: id người đang đăng nhập (đã qua middleware Auth).
func uid(r *http.Request) int64 {
	id, _ := middleware.UserIDFromContext(r.Context())
	return id
}

// bodyOr400 đọc JSON body; sai thì trả 400 và ok=false.
func bodyOr400(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := decodeJSON(w, r, dst); err != nil {
		response.Error(w, http.StatusBadRequest, "Dữ liệu gửi lên không hợp lệ.")
		return false
	}
	return true
}

// idsOf: "1,2,3" hoặc ids=1&ids=2 -> []int64
func idsOf(r *http.Request, key string) []int64 {
	var out []int64
	for _, v := range r.URL.Query()[key] {
		for _, p := range strings.Split(v, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil && id > 0 {
				out = append(out, id)
			}
		}
	}
	return out
}

func ok(w http.ResponseWriter, data any) { response.Success(w, http.StatusOK, data) }

// msg: phản hồi thao tác ghi chỉ có câu báo.
func msg(w http.ResponseWriter, m string) { ok(w, map[string]any{"message": m}) }
