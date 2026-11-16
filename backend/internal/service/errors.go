package service

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by services. Handlers map these to HTTP status codes.
// Bọc kèm câu báo cho người dùng: fail(ErrInvalidInput, "Thiếu ...") -> handler chỉ trả "Thiếu ...".
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("Sai số điện thoại / tên đăng nhập hoặc mật khẩu.")
	ErrConflict           = errors.New("Đã có tài khoản với thông tin này.")
	ErrNotFound           = errors.New("Không tìm thấy.")
	ErrForbidden          = errors.New("Bạn không có quyền với thao tác này.")
	ErrTooManyAttempts    = errors.New("Sai mật khẩu nhiều lần, thử lại sau 5 phút.")
	ErrLocked             = errors.New("Tài khoản đã bị khoá. Liên hệ Vua Thầu để được mở lại.")
	ErrUnauthorized       = errors.New("Hết phiên đăng nhập, đăng nhập lại nhé.")
	ErrUnavailable        = errors.New("Dịch vụ tạm thời không dùng được, thử lại sau.") // dịch vụ ngoài (tra địa chỉ...) lỗi
)

// UserError: lỗi kèm câu báo cho người dùng, vẫn so được bằng errors.Is với sentinel.
type UserError struct {
	Kind error
	Msg  string
}

func (e *UserError) Error() string { return e.Msg }
func (e *UserError) Unwrap() error { return e.Kind }

func fail(kind error, format string, args ...any) error {
	return &UserError{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// PublicMessage: câu báo an toàn để gửi cho người dùng.
func PublicMessage(err error) string {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Msg
	}
	for _, s := range []error{ErrInvalidCredentials, ErrConflict, ErrNotFound, ErrForbidden, ErrTooManyAttempts, ErrLocked, ErrUnauthorized} {
		if errors.Is(err, s) {
			return s.Error()
		}
	}
	return err.Error()
}

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
