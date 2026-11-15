package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

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
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrInvalidCredentials):
		response.Error(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrConflict):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	default:
		log.Printf("internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
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
