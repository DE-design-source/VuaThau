package service

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"

	"vuathau/backend/internal/integration/google"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/token"
	"vuathau/backend/internal/repository"
)

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcrypt.DefaultCost)

// RegisterInput: CTV đăng ký trên app. IDToken (Google) không bắt buộc.
type RegisterInput struct {
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Password  string `json:"password"`
	BankName  string `json:"bank_name"`
	BankNo    string `json:"bank_no"`
	BankOwner string `json:"bank_owner"`
	IDToken   string `json:"id_token"`
}

// PhoneResult: app đăng nhập bằng SĐT. Registered = false: SĐT chưa có tài khoản, app sang Đăng ký (điền sẵn SĐT).
type PhoneResult struct {
	*AuthResult
	Registered bool   `json:"registered"`
	Phone      string `json:"phone"`
}

type LoginInput struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type AuthResult struct {
	User        *model.User `json:"user"`
	AccessToken string      `json:"access_token"`
	ExpiresIn   int64       `json:"expires_in"` // seconds
	Message     string      `json:"message,omitempty"`
}

// GoogleResult: Google đã có tài khoản thì kèm AuthResult; chưa thì Registered = false để app sang Đăng ký.
type GoogleResult struct {
	*AuthResult
	Registered bool             `json:"registered"`
	Google     *google.Identity `json:"google,omitempty"`
}

type AuthService interface {
	Register(ctx context.Context, in RegisterInput) (*AuthResult, error)
	Login(ctx context.Context, in LoginInput) (*AuthResult, error)
	Google(ctx context.Context, idToken string) (*GoogleResult, error)
	// PhoneLogin: CTV vào app chỉ bằng SĐT (góp ý 20/11/2026), không mật khẩu.
	PhoneLogin(ctx context.Context, phone string) (*PhoneResult, error)
	// Issue cấp token mới cho người dùng (sau khi đổi mật khẩu).
	Issue(u *model.User) (*AuthResult, error)
}

// GoogleVerifier: xác minh id_token Google Sign-In.
type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) *google.Identity
}

type authService struct {
	repo      repository.AuthRepository
	google    GoogleVerifier
	jwtSecret string
	jwtExpiry time.Duration

	mu    sync.Mutex
	fails map[string]failCount // chống dò mật khẩu, theo tên đăng nhập

	upgrade func(ctx context.Context, id int64, hash string) error
}

type failCount struct {
	n     int
	since time.Time
}

const (
	maxFails   = 5
	failWindow = 5 * time.Minute
)

func NewAuthService(repo repository.AuthRepository, g GoogleVerifier, jwtSecret string, jwtExpiry time.Duration) AuthService {
	s := &authService{repo: repo, google: g, jwtSecret: jwtSecret, jwtExpiry: jwtExpiry, fails: map[string]failCount{}}
	if up, ok := repo.(interface {
		RehashPassword(ctx context.Context, id int64, hash string) error
	}); ok {
		s.upgrade = up.RehashPassword
	}
	return s
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

func checkPassword(hash, pw string) bool {
	if isLegacyHash(hash) {
		return checkLegacy(hash, pw)
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Mật khẩu chuyển từ máy chủ Python cũ: "<salt>$<hex scrypt(n=2^14, r=8, p=1, 64 byte)>".
func isLegacyHash(h string) bool {
	return !strings.HasPrefix(h, "$2") && strings.Count(h, "$") == 1
}

func checkLegacy(hash, pw string) bool {
	salt, want, _ := strings.Cut(hash, "$")
	got, err := scrypt.Key([]byte(pw), []byte(salt), 1<<14, 8, 1, 64)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(got)), []byte(want)) == 1
}

func (s *authService) Login(ctx context.Context, in LoginInput) (*AuthResult, error) {
	login := LoginID(in.Login)
	s.mu.Lock()
	f := s.fails[login]
	if f.n >= maxFails && time.Since(f.since) < failWindow {
		s.mu.Unlock()
		return nil, ErrTooManyAttempts
	}
	s.mu.Unlock()

	u, err := s.repo.FindByLogin(ctx, login)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if u == nil || u.Locked == model.LockedDeleted || !checkPassword(u.PasswordHash, in.Password) {
		if u == nil {
			// Tốn cùng thời gian bcrypt để không lộ tên đăng nhập nào tồn tại
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		}
		s.mu.Lock()
		if time.Since(f.since) >= failWindow {
			f = failCount{}
		}
		s.fails[login] = failCount{n: f.n + 1, since: time.Now()}
		s.mu.Unlock()
		return nil, ErrInvalidCredentials
	}
	s.mu.Lock()
	delete(s.fails, login)
	s.mu.Unlock()
	if u.Locked != model.LockedNone {
		return nil, ErrLocked
	}
	if isLegacyHash(u.PasswordHash) && s.upgrade != nil { // nâng mật khẩu cũ lên bcrypt, giữ nguyên token_version
		if h, err := hashPassword(in.Password); err == nil {
			_ = s.upgrade(ctx, u.ID, h)
		}
	}
	return s.Issue(u)
}

// PhoneLogin: SĐT có tài khoản CTV thì vào luôn; chưa có thì báo app sang Đăng ký. Tài khoản nhân viên vẫn dùng mật khẩu.
// Ai biết SĐT cũng vào được, nên CTV không tự đổi ngân hàng khi đã có (UserService.UpdateBank), phải nhờ admin.
func (s *authService) PhoneLogin(ctx context.Context, raw string) (*PhoneResult, error) {
	phone := normPhone(raw)
	if !ValidPhone(phone) {
		return nil, fail(ErrInvalidInput, "Số điện thoại chưa đúng, nhập 10 số, ví dụ 0912345678.")
	}
	u, err := s.repo.FindByLogin(ctx, phone)
	if errors.Is(err, repository.ErrNotFound) {
		return &PhoneResult{Registered: false, Phone: phone}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if u.Role != model.RoleCTV && u.Role != model.RoleNhaThau { // CTV và nhà thầu vào app bằng SĐT
		return nil, fail(ErrForbidden, "Tài khoản nhân viên đăng nhập trên web bằng mật khẩu.")
	}
	if u.Locked == model.LockedDeleted {
		return nil, fail(ErrLocked, "Tài khoản của số này đã xoá. Liên hệ admin Vua Thầu để mở lại.")
	}
	res, err := s.signIn(u, "")
	if err != nil {
		return nil, err
	}
	return &PhoneResult{AuthResult: res, Registered: true, Phone: phone}, nil
}

func (s *authService) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	phone := normPhone(in.Phone)
	name := truncate(in.Name, 100)
	var g *google.Identity
	if in.IDToken != "" {
		if g = s.google.Verify(ctx, in.IDToken); g == nil {
			return nil, fail(ErrInvalidInput, "Phiên Google đã hết hạn, bấm Đăng ký bằng Google lại nhé.")
		}
		if u, err := s.repo.FindByGoogle(ctx, g.Sub); err == nil {
			return s.signIn(u, "") // Google này đã đăng ký rồi: vào luôn
		}
		// CTV cũ (đăng ký bằng SĐT): gắn Google vào tài khoản đó (app vào bằng SĐT nên không hỏi mật khẩu cũ)
		if old, err := s.repo.FindByLogin(ctx, phone); err == nil {
			if old.Role != model.RoleCTV || old.GoogleSub != "" || old.Locked != model.LockedNone {
				return nil, fail(ErrConflict, "Số điện thoại này đã có tài khoản. Quay lại, nhập SĐT để đăng nhập.")
			}
			if err := s.repo.LinkGoogle(ctx, old.ID, g.Sub, g.Email); err != nil {
				return nil, fmt.Errorf("link google: %w", err)
			}
			return s.signIn(old, "Đã liên kết Google với tài khoản của bạn. Lần sau bấm Đăng nhập bằng Google.")
		}
	}
	bank := BankFrom(in.BankName, in.BankNo, in.BankOwner)
	// mật khẩu không bắt buộc nữa (app vào bằng SĐT); không nhập thì đặt mật khẩu ngẫu nhiên
	pw := in.Password
	if pw == "" {
		pw = randomSecret()
	}
	if !ValidPhone(phone) || utf8.RuneCountInString(pw) < 6 || len(pw) > 72 || name == "" || bank == "" {
		return nil, fail(ErrInvalidInput,
			"Kiểm tra lại: SĐT hợp lệ, có họ tên, đủ ngân hàng, số tài khoản (chỉ số) và tên chủ tài khoản.")
	}
	hash, err := hashPassword(pw)
	if err != nil {
		return nil, err
	}
	u := &model.User{Username: phone, Name: name, Role: model.RoleCTV, PasswordHash: hash, Bank: bank}
	if g != nil {
		u.GoogleSub, u.Email = g.Sub, g.Email
	}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, fail(ErrConflict, "Số điện thoại này đã có tài khoản.")
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return s.signIn(u, "Chào mừng bạn! Chụp công trình bằng camera Vua Thầu rồi nộp hồ sơ đầu tiên.")
}

func (s *authService) Google(ctx context.Context, idToken string) (*GoogleResult, error) {
	g := s.google.Verify(ctx, idToken)
	if g == nil {
		return nil, fail(ErrInvalidInput, "Không xác minh được tài khoản Google, thử lại nhé.")
	}
	u, err := s.repo.FindByGoogle(ctx, g.Sub)
	if errors.Is(err, repository.ErrNotFound) {
		return &GoogleResult{Registered: false, Google: g}, nil // chưa đăng ký: app sang Đăng ký, điền sẵn tên + email
	}
	if err != nil {
		return nil, err
	}
	res, err := s.signIn(u, "")
	if err != nil {
		return nil, err
	}
	return &GoogleResult{AuthResult: res, Registered: true}, nil
}

func (s *authService) signIn(u *model.User, message string) (*AuthResult, error) {
	if u.Locked != model.LockedNone {
		return nil, ErrLocked
	}
	res, err := s.Issue(u)
	if err != nil {
		return nil, err
	}
	res.Message = message
	return res, nil
}

func (s *authService) Issue(u *model.User) (*AuthResult, error) {
	tok, err := token.Generate(s.jwtSecret, u.ID, u.TokenVersion, s.jwtExpiry)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	return &AuthResult{User: u, AccessToken: tok, ExpiresIn: int64(s.jwtExpiry.Seconds())}, nil
}
