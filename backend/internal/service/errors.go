package service

import "errors"

// Sentinel errors returned by services. Handlers map these to HTTP status codes.
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrConflict           = errors.New("email or username already in use")
	ErrNotFound           = errors.New("not found")
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
