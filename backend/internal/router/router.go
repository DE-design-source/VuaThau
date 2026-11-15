package router

import (
	"context"
	"net/http"
	"time"

	"vuathau/backend/internal/config"
	"vuathau/backend/internal/handler"
	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/websocket"
)

type HealthCheck func(ctx context.Context) error

type Deps struct {
	Config         *config.Config
	AuthHandler    *handler.AuthHandler
	UserHandler    *handler.UserHandler
	MessageHandler *handler.MessageHandler
	Hub            *websocket.Hub
	HealthChecks   map[string]HealthCheck // e.g. "postgres", "redis"
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	auth := middleware.Auth(d.Config.JWTSecret)
	protected := func(fn http.HandlerFunc) http.Handler { return auth(fn) }

	mux.HandleFunc("GET /health", healthHandler(d.HealthChecks))

	// Auth
	mux.HandleFunc("POST /api/v1/auth/register", d.AuthHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", d.AuthHandler.Login)

	// Users
	mux.Handle("GET /api/v1/users/me", protected(d.UserHandler.Me))
	mux.Handle("GET /api/v1/users", protected(d.UserHandler.List))
	mux.Handle("GET /api/v1/users/{id}", protected(d.UserHandler.GetByID))

	// Messages
	mux.Handle("POST /api/v1/messages", protected(d.MessageHandler.Send))
	mux.Handle("GET /api/v1/messages/{userID}", protected(d.MessageHandler.Conversation))

	// WebSocket: ws://host/ws?token=<jwt>
	mux.Handle("GET /ws", protected(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFromContext(r.Context())
		d.Hub.ServeWS(w, r, uid)
	}))

	// Unknown routes get the standard JSON envelope instead of plain text.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})

	// Outermost first: CORS -> rate limit -> routes.
	var h http.Handler = mux
	h = middleware.RateLimit(d.Config.RateLimitRPS, d.Config.RateLimitBurst)(h)
	h = middleware.CORS(d.Config.CORSAllowedOrigins)(h)
	return h
}

func healthHandler(checks map[string]HealthCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		healthy := true
		services := make(map[string]string, len(checks))
		for name, check := range checks {
			if err := check(ctx); err != nil {
				services[name] = "down"
				healthy = false
			} else {
				services[name] = "up"
			}
		}

		status, code := "ok", http.StatusOK
		if !healthy {
			status, code = "degraded", http.StatusServiceUnavailable
		}
		response.JSON(w, code, response.Body{
			Success: healthy,
			Data:    map[string]any{"status": status, "services": services},
		})
	}
}
