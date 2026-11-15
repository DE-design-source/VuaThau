package middleware

import (
	"context"
	"net/http"
	"strings"

	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/pkg/token"
)

type ctxKey int

const userIDKey ctxKey = iota

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
				response.Error(w, http.StatusUnauthorized, "missing access token")
				return
			}

			claims, err := token.Parse(jwtSecret, tok)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid or expired access token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
