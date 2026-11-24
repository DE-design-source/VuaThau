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
	"vuathau/backend/internal/integration/dezon"
	"vuathau/backend/internal/middleware"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/response"
)

type HealthCheck func(ctx context.Context) error

type Deps struct {
	Config         *config.Config
	AuthHandler    *handler.AuthHandler
	UserHandler    *handler.UserHandler
	HouseHandler   *handler.HouseHandler
	PaymentHandler *handler.PaymentHandler
	UserLookup     middleware.UserLookup
	HealthChecks   map[string]HealthCheck // e.g. "postgres", "redis"
	Tiles          *handler.TileHandler   // nền bản đồ qua máy chủ; nil: tạo mới theo DATA_DIR
	Dezon          *handler.DezonHandler  // dữ liệu dezon.vn; nil: chỉ nhà thầu tiềm năng, không có công trình
	Contractors    *handler.ContractorHandler // trang Nhà thầu (admin); nil: tắt
	NT             *handler.NTHandler     // tài khoản nhà thầu trên app; nil: tắt
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	auth := middleware.Auth(d.Config.JWTSecret)
	// role: đăng nhập + nạp người dùng (chặn token cũ, tài khoản khoá) + đúng vai trò (trống = mọi vai trò)
	role := func(fn http.HandlerFunc, roles ...string) http.Handler {
		return auth(middleware.Session(d.UserLookup, roles...)(fn))
	}
	const ctv, admin, ketoan, sale = model.RoleCTV, model.RoleAdmin, model.RoleKetoan, model.RoleSale

	mux.HandleFunc("GET /health", healthHandler(d.HealthChecks))

	a, u, h, p := d.AuthHandler, d.UserHandler, d.HouseHandler, d.PaymentHandler

	// Công khai
	mux.HandleFunc("GET /api/v1/config", p.Config)
	mux.HandleFunc("GET /api/v1/geo/provinces", h.Provinces)
	// Bản đồ công khai (người chưa đăng nhập): không tên CTV, ảnh đại diện đã che SĐT qua link có chữ ký
	mux.HandleFunc("GET /api/v1/public/map", h.PublicMap)
	mux.HandleFunc("GET /api/v1/public/photos/{pid}", h.PublicPhoto)
	mux.HandleFunc("GET /api/v1/public/projects/{id}", h.PublicProject)
	if nt := d.Contractors; nt != nil {
		mux.Handle("GET /api/v1/contractors", role(nt.List, model.RoleAdmin, model.RoleKetoan))
		mux.Handle("GET /api/v1/contractors/{key}", role(nt.Get, model.RoleAdmin, model.RoleKetoan))
		mux.Handle("POST /api/v1/contractors/sync", role(nt.Sync, model.RoleAdmin))
	}
	mux.HandleFunc("GET /api/v1/public/geo/search", h.PublicSearchPlace)
	mux.HandleFunc("GET /api/v1/geo/divisions", handler.Divisions) // 34 tỉnh + phường / xã 2025 (form Đăng dự án)

	// Tải app Android (link gửi qua Zalo) + cấu hình cho app Expo cũ (để nó mời tải app mới)
	dl := handler.NewDownloadHandler(d.Config.DataDir, p)
	mux.HandleFunc("GET /tai-app", dl.Page)
	mux.HandleFunc("GET /tai-app/vuathau.apk", dl.APK)
	mux.HandleFunc("GET /tai-app/phien-ban", dl.Version)
	mux.HandleFunc("GET /app.json", dl.LegacyConfig)
	mux.HandleFunc("POST /api/v1/auth/register", a.Register)
	mux.HandleFunc("POST /api/v1/auth/login", a.Login)
	mux.HandleFunc("POST /api/v1/auth/google", a.Google)
	mux.HandleFunc("POST /api/v1/auth/phone", a.Phone)

	// Nhà thầu trên app: hồ sơ công ty, dự án, nhật ký thi công (đợt 1, 23/11/2026)
	if nt := d.NT; nt != nil {
		nth := model.RoleNhaThau
		mux.Handle("GET /api/v1/nt/home", role(nt.Home, nth))
		mux.Handle("PUT /api/v1/nt/profile", role(nt.SaveProfile, nth))
		mux.Handle("GET /api/v1/nt/projects", role(nt.Projects, nth))
		mux.Handle("POST /api/v1/nt/projects", role(nt.CreateProject, nth))
		mux.Handle("GET /api/v1/nt/projects/{id}", role(nt.Project, nth, model.RoleAdmin))
		mux.Handle("PUT /api/v1/nt/projects/{id}", role(nt.UpdateProject, nth))
		mux.Handle("DELETE /api/v1/nt/projects/{id}", role(nt.DeleteProject, nth))
		mux.Handle("POST /api/v1/nt/projects/{id}/photos", role(nt.AddProjectPhotos, nth))
		mux.Handle("POST /api/v1/nt/projects/{id}/logs", role(nt.AddLog, nth))
		mux.Handle("DELETE /api/v1/nt/projects/{id}/logs/{lid}", role(nt.DeleteLog, nth))
		mux.Handle("GET /api/v1/nt/projects/{id}/logs/{lid}/report", role(nt.Report, nth))
		// báo cáo gửi chủ nhà: link công khai /bc/<mã> (mã có chữ ký), không cần đăng nhập
		mux.HandleFunc("GET /api/v1/public/bc/{code}", nt.PublicReport)
		mux.HandleFunc("GET /api/v1/public/bc/{code}/photos/{mid}", nt.PublicReportPhoto)
		mux.Handle("GET /api/v1/nt/photos/{id}", role(nt.Photo, nth, model.RoleAdmin))
		mux.HandleFunc("GET /api/v1/nt/videos/{id}", nt.Video)
		mux.Handle("GET /api/v1/admin/nt", role(nt.Contractors, model.RoleAdmin))
		mux.Handle("POST /api/v1/admin/nt", role(nt.AddContractor, model.RoleAdmin))
	}

	// Tài khoản (mọi vai trò)
	mux.Handle("GET /api/v1/me", role(u.Me))
	mux.Handle("GET /api/v1/users/me", role(u.Me))
	mux.Handle("PUT /api/v1/me/bank", role(u.UpdateBank, ctv))
	mux.Handle("PUT /api/v1/me/password", role(u.ChangePassword))
	mux.Handle("DELETE /api/v1/me", role(u.DeleteSelf, ctv))

	// CTV
	mux.Handle("GET /api/v1/ctv/home", role(h.Home, ctv))
	mux.Handle("GET /api/v1/ctv/income", role(h.Income, ctv))
	mux.Handle("GET /api/v1/ctv/map", role(h.CTVMap, ctv))
	mux.Handle("GET /api/v1/geo/search", role(h.SearchPlace, ctv, admin, ketoan, sale))
	mux.Handle("GET /api/v1/geocode", role(h.Geocode, ctv, model.RoleNhaThau)) // nhà thầu: Ghim trên bản đồ (chỉ trả địa chỉ)
	mux.Handle("POST /api/v1/photos/check", role(h.CheckPhoto, ctv))
	mux.Handle("POST /api/v1/houses", role(h.Create, ctv))
	mux.Handle("PUT /api/v1/houses/{id}", role(h.Save, ctv))
	mux.Handle("POST /api/v1/houses/{id}/photos", role(h.AddPhotos, ctv))
	mux.Handle("DELETE /api/v1/houses/{id}/photos/{pid}", role(h.DeletePhoto, ctv))
	mux.Handle("POST /api/v1/houses/{id}/videos", role(h.AddVideos, ctv))
	mux.Handle("DELETE /api/v1/houses/{id}/videos/{vid}", role(h.DeleteVideo, ctv))
	mux.HandleFunc("GET /api/v1/videos/{vid}", h.Video) // link có chữ ký, không cần đăng nhập

	// Hồ sơ: CTV chủ hồ sơ, admin, kế toán (service kiểm quyền xem)
	// (không cho sale: có tên CTV, tiền)
	mux.Handle("GET /api/v1/houses/{id}", role(h.Detail, ctv, admin, ketoan))
	mux.Handle("GET /api/v1/houses/{id}/state", role(h.State, ctv, admin, ketoan))
	mux.Handle("GET /api/v1/photos/{pid}", role(h.Photo)) // sale: chỉ ảnh của dự án được xem
	mux.Handle("GET /api/v1/batches/{id}", role(p.Batch, ctv, admin, ketoan))
	mux.Handle("GET /api/v1/batches/{id}/receipt", role(p.Receipt, ctv, admin, ketoan))

	// Sale: bản đồ, dự án, nhà thầu. Không có CTV, không có tiền. Admin xem được để kiểm tra.
	mux.Handle("GET /api/v1/sale/map", role(h.SaleMap, sale, admin))
	mux.Handle("GET /api/v1/sale/projects", role(h.SaleProjects, sale, admin))
	mux.Handle("GET /api/v1/sale/projects/{id}", role(h.SaleProject, sale, admin))
	mux.Handle("GET /api/v1/sale/contractors", role(h.Contractors, sale, admin))
	dz := d.Dezon
	if dz == nil {
		dz = handler.NewDezonHandler(dezon.ListURL, nil)
	}
	mux.Handle("GET /api/v1/sale/dezon-contractors", role(dz.Contractors, sale, admin))
	// Công trình nhà thầu đăng trên dezon.vn: lớp ghim riêng trên bản đồ admin / kế toán / sale
	mux.Handle("GET /api/v1/dezon/projects", role(dz.Projects, sale, admin, ketoan))

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
	mux.Handle("GET /api/v1/admin/inbox", role(h.InboxStatus, admin))
	mux.Handle("POST /api/v1/admin/inbox/sync", role(h.InboxSync, admin))
	mux.Handle("PUT /api/v1/settings", role(p.SaveSettings, admin))
	// Khách hàng (lấy từ pipeline Kinh doanh của Dezon Pro, chỉ xem)
	mux.Handle("GET /api/v1/customers", role(h.Customers, admin, ketoan))
	// Quy tắc duyệt ảnh cho AI (admin viết, gửi kèm mỗi lượt AI đọc ảnh)
	mux.Handle("GET /api/v1/settings/ai-rules", role(h.AIRules, admin))
	mux.Handle("PUT /api/v1/settings/ai-rules", role(h.SaveAIRules, admin))
	mux.Handle("PUT /api/v1/settings/photo-order", role(h.SavePhotoOrder, admin))

	// Admin + kế toán
	mux.Handle("PUT /api/v1/houses/{id}/amount", role(h.SetAmount, admin, ketoan))
	mux.Handle("PUT /api/v1/houses/{id}/scope", role(h.SetScope, admin))
	mux.Handle("POST /api/v1/houses/{id}/split", role(h.Split, admin))
	mux.Handle("GET /api/v1/houses", role(h.Overview, admin, ketoan))
	mux.Handle("GET /api/v1/ctvs", role(u.CTVs, admin, ketoan))
	mux.Handle("POST /api/v1/ctvs", role(u.AddCTV, admin))
	mux.Handle("GET /api/v1/map", role(h.Map, admin, ketoan))
	mux.Handle("GET /api/v1/ctvs/{id}", role(u.CTV, admin, ketoan))
	mux.Handle("PUT /api/v1/ctvs/{id}/profile", role(u.CTVProfile, admin))
	mux.Handle("PUT /api/v1/ctvs/{id}/login", role(u.CTVLogin, admin))
	mux.Handle("PUT /api/v1/ctvs/{id}/bank", role(u.CTVBank, admin))
	mux.Handle("PUT /api/v1/ctv-kinds", role(u.CTVKinds, admin))
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

	// API lạ: envelope JSON. Còn lại: web React (nếu STATIC_DIR) hoặc 404.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})
	mux.Handle("/", spa(d.Config.StaticDir))

	// Outermost first: CORS -> rate limit -> routes.
	var hd http.Handler = mux
	hd = middleware.RateLimit(d.Config.RateLimitRPS, d.Config.RateLimitBurst)(hd)
	hd = middleware.Slow(2 * time.Second)(hd) // ghi nhật ký yêu cầu chậm để đo chỗ nghẽn
	hd = middleware.CORS(d.Config.CORSAllowedOrigins)(hd)

	// Ô nền bản đồ: 1 lần mở bản đồ tải vài chục ô cùng lúc, không tính vào giới hạn tốc độ của API
	tiles := http.NewServeMux()
	tl := d.Tiles
	if tl == nil {
		tl = handler.NewTileHandler(d.Config.DataDir)
	}
	tiles.HandleFunc("GET /tiles/{z}/{x}/{file}", tl.Serve)
	tiles.HandleFunc("GET /map-style/vn.json", handler.NewBasemapHandler(d.Config.VietmapKey).Style)
	limited := hd
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/tiles/") || r.URL.Path == "/map-style/vn.json" {
			tiles.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
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
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() {
			switch {
			case strings.HasPrefix(r.URL.Path, "/assets/"):
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			case strings.HasPrefix(r.URL.Path, "/app/"):
				// App CTV bản web (Flutter, tên file không đổi theo bản): luôn hỏi lại máy chủ để nhận bản mới
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		// Thư mục con có index.html riêng (app CTV bản web ở /app/): trả index.html của nó
		if err == nil && st.IsDir() && r.URL.Path != "/" {
			if html, err := os.ReadFile(filepath.Join(p, "index.html")); err == nil {
				if !strings.HasSuffix(r.URL.Path, "/") {
					http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = w.Write(html)
				return
			}
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
