package model

import "time"

// Vai trò người dùng.
const (
	RoleCTV     = "ctv"     // cộng tác viên: dùng app điện thoại
	RoleAdmin   = "admin"   // duyệt hồ sơ, quản lý
	RoleKetoan  = "ketoan"  // kế toán: thanh toán
	RoleSale    = "sale"    // sale: chỉ xem bản đồ, dự án, nhà thầu (không thấy CTV, không thấy tiền)
	RoleNhaThau = "nhathau" // nhà thầu: app, tự đăng dự án + nhật ký thi công (đợt 1, 23/11/2026)
)

// Trạng thái khoá tài khoản.
const (
	LockedNone    = 0
	LockedByAdmin = 1
	LockedDeleted = 2 // CTV tự xoá tài khoản
	// LockedUnverified: nhà thầu bị bỏ tích "Xác minh tham gia" trên form Lark (tự mở lại khi tích lại)
	LockedUnverified = 3
)

// User: CTV, admin hoặc kế toán. Username là tên đăng nhập (SĐT với CTV).
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Email        string    `json:"email,omitempty"`
	Bank         string    `json:"bank"`
	Locked       int       `json:"locked"`
	GoogleSub    string    `json:"-"`
	TokenVersion int       `json:"-"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *User) IsStaff() bool { return u.Role == RoleAdmin || u.Role == RoleKetoan }

// Bank: tài khoản nhận tiền, lưu gộp trong User.Bank dạng "Ngân hàng · Số TK · CHỦ TK".
type Bank struct {
	Name  string `json:"name"`
	No    string `json:"no"`
	Owner string `json:"owner"`
}
