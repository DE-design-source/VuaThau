package middleware

import (
	"context"
	"net/http"
	"strings"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/pkg/token"
)

type ctxKey int

const (
	userIDKey ctxKey = iota
	tokenVerKey
	userKey
)

// Auth validates the JWT and stores the user id in the request context.
// The token is read from "Authorization: Bearer <token>"; for WebSocket
// upgrade requests (browsers can't set headers) it may be passed as ?token=.
func Auth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearerToken(r)
			if tok == "" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				tok = r.URL.Query().Get("token")
			}
			if tok == "" {
				response.Error(w, http.StatusUnauthorized, "Chưa đăng nhập.")
				return
			}

			claims, err := token.Parse(jwtSecret, tok)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "Hết phiên đăng nhập, đăng nhập lại nhé.")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, tokenVerKey, claims.Ver)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserLookup đọc người dùng theo id (repository).
type UserLookup func(ctx context.Context, id int64) (*model.User, error)

// Session (sau Auth): nạp người dùng, chặn token cũ (đã đổi mật khẩu / bị khoá) và tài khoản bị khoá.
// roles: chỉ cho các vai trò này (trống = mọi vai trò).
func Session(lookup UserLookup, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := UserIDFromContext(r.Context())
			ver, _ := r.Context().Value(tokenVerKey).(int)
			u, err := lookup(r.Context(), id)
			if err != nil || u == nil || u.TokenVersion != ver || u.Locked == model.LockedDeleted {
				response.Error(w, http.StatusUnauthorized, "Hết phiên đăng nhập, đăng nhập lại nhé.")
				return
			}
			if u.Locked != model.LockedNone {
				response.Error(w, http.StatusForbidden, "Tài khoản đã bị khoá. Liên hệ Vua Thầu để được mở lại.")
				return
			}
			if len(roles) > 0 {
				allowed := false
				for _, role := range roles {
					allowed = allowed || u.Role == role
				}
				if !allowed {
					response.Error(w, http.StatusForbidden, "Bạn không có quyền với trang này.")
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
		})
	}
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

// UserFromContext: người đang đăng nhập (sau middleware Session).
func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
