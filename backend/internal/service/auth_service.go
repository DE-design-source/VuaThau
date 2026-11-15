package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/token"
	"vuathau/backend/internal/repository"
)

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcrypt.DefaultCost)

type RegisterInput struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResult struct {
	User        *model.User `json:"user"`
	AccessToken string      `json:"access_token"`
	ExpiresIn   int64       `json:"expires_in"` // seconds
}

type AuthService interface {
	Register(ctx context.Context, in RegisterInput) (*AuthResult, error)
	Login(ctx context.Context, in LoginInput) (*AuthResult, error)
}

type authService struct {
	repo      repository.AuthRepository
	jwtSecret string
	jwtExpiry time.Duration
}

func NewAuthService(repo repository.AuthRepository, jwtSecret string, jwtExpiry time.Duration) AuthService {
	return &authService{repo: repo, jwtSecret: jwtSecret, jwtExpiry: jwtExpiry}
}

func (s *authService) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	if n := utf8.RuneCountInString(in.Username); n < 3 || n > 50 {
		return nil, fmt.Errorf("%w: username must be 3-50 characters", ErrInvalidInput)
	}
	// Reject display-name forms like "Bob <a@b.com>": only a bare address is allowed.
	if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email || len(in.Email) > 255 {
		return nil, fmt.Errorf("%w: email is not valid", ErrInvalidInput)
	}
	// bcrypt only uses the first 72 bytes.
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return nil, fmt.Errorf("%w: password must be 8-72 characters", ErrInvalidInput)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &model.User{Username: in.Username, Email: in.Email, PasswordHash: string(hash)}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	return s.issue(u)
}

func (s *authService) Login(ctx context.Context, in LoginInput) (*AuthResult, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))

	u, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Spend the same bcrypt time so response timing doesn't reveal which emails exist.
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issue(u)
}

func (s *authService) issue(u *model.User) (*AuthResult, error) {
	tok, err := token.Generate(s.jwtSecret, u.ID, s.jwtExpiry)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	return &AuthResult{User: u, AccessToken: tok, ExpiresIn: int64(s.jwtExpiry.Seconds())}, nil
}
