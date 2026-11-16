package router

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"vuathau/backend/internal/config"
	"vuathau/backend/internal/handler"
	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/response"
	"vuathau/backend/internal/websocket"
)

type HealthCheck func(ctx context.Context) error

type Deps struct {
	Config         *config.Config
	AuthHandler    *handler.AuthHandler
	UserHandler    *handler.UserHandler
	MessageHandler *handler.MessageHandler
	HouseHandler   *handler.HouseHandler
	PaymentHandler *handler.PaymentHandler
	UserLookup     middleware.UserLookup
	Hub            *websocket.Hub
	HealthChecks   map[string]HealthCheck // e.g. "postgres", "redis"
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	auth := middleware.Auth(d.Config.JWTSecret)
	protected := func(fn http.HandlerFunc) http.Handler { return auth(fn) }
	// role: đăng nhập + nạp người dùng (chặn token cũ, tài khoản khoá) + đúng vai trò (trống = mọi vai trò)
	role := func(fn http.HandlerFunc, roles ...string) http.Handler {
		return auth(middleware.Session(d.UserLookup, roles...)(fn))
	}
	const ctv, admin, ketoan = model.RoleCTV, model.RoleAdmin, model.RoleKetoan

	mux.HandleFunc("GET /health", healthHandler(d.HealthChecks))

	a, u, h, p := d.AuthHandler, d.UserHandler, d.HouseHandler, d.PaymentHandler

	// Công khai
	mux.HandleFunc("GET /api/v1/config", p.Config)

	// Tải app Android (link gửi qua Zalo) + cấu hình cho app Expo cũ (để nó mời tải app mới)
	dl := handler.NewDownloadHandler(d.Config.DataDir, p)
	mux.HandleFunc("GET /tai-app", dl.Page)
	mux.HandleFunc("GET /tai-app/vuathau.apk", dl.APK)
	mux.HandleFunc("GET /tai-app/phien-ban", dl.Version)
	mux.HandleFunc("GET /app.json", dl.LegacyConfig)
	mux.HandleFunc("POST /api/v1/auth/register", a.Register)
	mux.HandleFunc("POST /api/v1/auth/login", a.Login)
	mux.HandleFunc("POST /api/v1/auth/google", a.Google)

	// Tài khoản (mọi vai trò)
	mux.Handle("GET /api/v1/me", role(u.Me))
	mux.Handle("GET /api/v1/users/me", role(u.Me))
	mux.Handle("PUT /api/v1/me/bank", role(u.UpdateBank, ctv))
	mux.Handle("PUT /api/v1/me/password", role(u.ChangePassword))
	mux.Handle("DELETE /api/v1/me", role(u.DeleteSelf, ctv))

	// CTV
	mux.Handle("GET /api/v1/ctv/home", role(h.Home, ctv))
	mux.Handle("GET /api/v1/ctv/income", role(h.Income, ctv))
	mux.Handle("GET /api/v1/geocode", role(h.Geocode, ctv))
	mux.Handle("POST /api/v1/photos/check", role(h.CheckPhoto, ctv))
	mux.Handle("POST /api/v1/houses", role(h.Create, ctv))
	mux.Handle("PUT /api/v1/houses/{id}", role(h.Save, ctv))
	mux.Handle("POST /api/v1/houses/{id}/photos", role(h.AddPhotos, ctv))
	mux.Handle("DELETE /api/v1/houses/{id}/photos/{pid}", role(h.DeletePhoto, ctv))

	// Hồ sơ: CTV chủ hồ sơ, admin, kế toán (service kiểm quyền xem)
	mux.Handle("GET /api/v1/houses/{id}", role(h.Detail))
	mux.Handle("GET /api/v1/houses/{id}/state", role(h.State))
	mux.Handle("GET /api/v1/photos/{pid}", role(h.Photo))
	mux.Handle("GET /api/v1/batches/{id}", role(p.Batch))
	mux.Handle("GET /api/v1/batches/{id}/receipt", role(p.Receipt))

	// Admin
	mux.Handle("GET /api/v1/admin/queue", role(h.Queue, admin))
	mux.Handle("POST /api/v1/admin/approve", role(h.ApproveMany, admin))
	mux.Handle("POST /api/v1/houses/{id}/review", role(h.Review, admin))
	mux.Handle("POST /api/v1/houses/{id}/undo", role(h.Undo, admin))
	mux.Handle("PUT /api/v1/houses/{id}/info", role(h.EditInfo, admin))
	mux.Handle("GET /api/v1/staff", role(u.Staff, admin))
	mux.Handle("POST /api/v1/staff", role(u.AddStaff, admin))
	mux.Handle("POST /api/v1/users/{id}/lock", role(u.Lock, admin))
	mux.Handle("POST /api/v1/users/{id}/password", role(u.ResetPassword, admin))
	mux.Handle("GET /api/v1/settings", role(p.Settings, admin))
	mux.Handle("PUT /api/v1/settings", role(p.SaveSettings, admin))

	// Admin + kế toán
	mux.Handle("PUT /api/v1/houses/{id}/amount", role(h.SetAmount, admin, ketoan))
	mux.Handle("GET /api/v1/houses", role(h.Overview, admin, ketoan))
	mux.Handle("GET /api/v1/ctvs", role(u.CTVs, admin, ketoan))
	mux.Handle("GET /api/v1/ctvs/{id}", role(u.CTV, admin, ketoan))
	mux.Handle("POST /api/v1/ctvs/{id}/adjustments", role(p.AddAdjustment, admin, ketoan))
	mux.Handle("DELETE /api/v1/adjustments/{id}", role(p.DeleteAdjustment, admin, ketoan))
	mux.Handle("GET /api/v1/batches", role(p.Batches, admin, ketoan))
	mux.Handle("GET /api/v1/reports", role(p.Report, admin, ketoan))
	mux.Handle("GET /api/v1/reports/export", role(p.ReportExport, admin, ketoan))

	// Kế toán
	mux.Handle("GET /api/v1/accounting", role(p.Accounting, ketoan))
	mux.Handle("GET /api/v1/accounting/export", role(p.Export, ketoan))
	mux.Handle("POST /api/v1/accounting/pay", role(p.PayMany, ketoan))
	mux.Handle("GET /api/v1/payouts/{ctv_id}", role(p.Payout, ketoan))
	mux.Handle("POST /api/v1/payouts/{ctv_id}", role(p.Pay, ketoan))
	mux.Handle("PUT /api/v1/batches/{id}", role(p.SetRef, ketoan))
	mux.Handle("POST /api/v1/batches/{id}/cancel", role(p.Cancel, ketoan))
	mux.Handle("POST /api/v1/batches/{id}/receipt", role(p.UploadReceipt, ketoan))

	// Scaffold: người dùng, tin nhắn, WebSocket
	mux.Handle("GET /api/v1/users", role(u.List))
	mux.Handle("GET /api/v1/users/{id}", role(u.GetByID))
	mux.Handle("POST /api/v1/messages", protected(d.MessageHandler.Send))
	mux.Handle("GET /api/v1/messages/{userID}", protected(d.MessageHandler.Conversation))

	// WebSocket: ws://host/ws?token=<jwt>
	mux.Handle("GET /ws", protected(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.UserIDFromContext(r.Context())
		d.Hub.ServeWS(w, r, uid)
	}))

	// API lạ: envelope JSON. Còn lại: web React (nếu STATIC_DIR) hoặc 404.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})
	mux.Handle("/", spa(d.Config.StaticDir))

	// Outermost first: CORS -> rate limit -> routes.
	var hd http.Handler = mux
	hd = middleware.RateLimit(d.Config.RateLimitRPS, d.Config.RateLimitBurst)(hd)
	hd = middleware.CORS(d.Config.CORSAllowedOrigins)(hd)
	return hd
}

// spa phục vụ web React đã build: file có thật thì trả file, đường dẫn lạ trả index.html (React Router xử lý).
func spa(dir string) http.Handler {
	if dir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.Error(w, http.StatusNotFound, "route not found")
		})
	}
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		// File tĩnh không có (hoặc không đọc được): 404 thật, không trả index.html dưới URL của file JS/CSS
		if strings.HasPrefix(r.URL.Path, "/assets/") || path.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		// Đường dẫn của React Router: trả index.html, không kèm Last-Modified / ETag để trình duyệt luôn lấy bản mới
		html, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			response.Error(w, http.StatusNotFound, "route not found")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(html)
	})
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
