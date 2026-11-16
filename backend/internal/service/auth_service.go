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

func (s *authService) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	phone := CleanPhone(strings.TrimSpace(in.Phone))
	name := truncate(in.Name, 100)
	var g *google.Identity
	if in.IDToken != "" {
		if g = s.google.Verify(ctx, in.IDToken); g == nil {
			return nil, fail(ErrInvalidInput, "Phiên Google đã hết hạn, bấm Đăng ký bằng Google lại nhé.")
		}
		if u, err := s.repo.FindByGoogle(ctx, g.Sub); err == nil {
			return s.signIn(u, "") // Google này đã đăng ký rồi: vào luôn
		}
		// CTV cũ (đăng ký bằng SĐT): đúng mật khẩu thì gắn Google vào tài khoản đó
		if old, err := s.repo.FindByLogin(ctx, phone); err == nil {
			if old.Role != model.RoleCTV || old.GoogleSub != "" || !checkPassword(old.PasswordHash, in.Password) {
				return nil, fail(ErrConflict, "Số điện thoại này đã có tài khoản. Nhập đúng mật khẩu cũ để liên kết Google.")
			}
			if err := s.repo.LinkGoogle(ctx, old.ID, g.Sub, g.Email); err != nil {
				return nil, fmt.Errorf("link google: %w", err)
			}
			return s.signIn(old, "Đã liên kết Google với tài khoản của bạn. Lần sau bấm Đăng nhập bằng Google.")
		}
	}
	bank := BankFrom(in.BankName, in.BankNo, in.BankOwner)
	if !ValidPhone(phone) || utf8.RuneCountInString(in.Password) < 6 || len(in.Password) > 72 || name == "" || bank == "" {
		return nil, fail(ErrInvalidInput,
			"Kiểm tra lại: SĐT hợp lệ, mật khẩu từ 6 ký tự, đủ ngân hàng, số tài khoản (chỉ số) và tên chủ tài khoản.")
	}
	hash, err := hashPassword(in.Password)
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
