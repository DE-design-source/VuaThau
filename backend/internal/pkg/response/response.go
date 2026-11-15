package response

import (
	"encoding/json"
	"net/http"
)

// Body is the standard JSON envelope for every API response.
type Body struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Success(w http.ResponseWriter, status int, data any) {
	JSON(w, status, Body{Success: true, Data: data})
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, Body{Success: false, Error: message})
}
