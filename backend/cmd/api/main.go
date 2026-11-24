package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vuathau/backend/internal/config"
	"vuathau/backend/internal/database"
	"vuathau/backend/internal/handler"
	"vuathau/backend/internal/integration/ai"
	"vuathau/backend/internal/integration/dezon"
	"vuathau/backend/internal/integration/dezontech"
	"vuathau/backend/internal/integration/lark"
	"vuathau/backend/internal/integration/gdrive"
	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/integration/google"
	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/integration/ocr"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/router"
	"vuathau/backend/internal/service"
	"vuathau/backend/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Mọi thời gian trả về theo giờ Việt Nam (RFC 3339 +07:00)
	time.Local = service.VN

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Infrastructure
	db, err := database.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Redis chỉ dùng cho /health. REDIS_ADDR=none: không dùng, /health chỉ kiểm Postgres.
	// Có cấu hình mà không kết nối được: vẫn chạy, /health báo redis "down".
	checks := map[string]router.HealthCheck{"postgres": db.Ping}
	if cfg.RedisAddr != "none" {
		if rdb, err := database.NewRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB); err != nil {
			log.Printf("Redis: %v (bỏ qua, API vẫn chạy)", err)
			checks["redis"] = func(context.Context) error { return errors.New("redis not connected") }
		} else {
			defer rdb.Close()
			checks["redis"] = func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
		}
	}

	files, err := storage.New(cfg.DataDir)
	if err != nil {
		return err
	}

	// Repository -> Service -> Handler
	authRepo := repository.NewAuthRepository(db)
	userRepo := repository.NewUserRepository(db)
	houseRepo := repository.NewHouseRepository(db)
	photoRepo := repository.NewPhotoRepository(db)
	logRepo := repository.NewLogRepository(db)
	payRepo := repository.NewPaymentRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)

	gVerifier := google.NewVerifier(cfg.GoogleClientIDs)
	var drive *gdrive.Client
	if cfg.DriveEnabled() {
		drive = gdrive.New(cfg.GDriveClientID, cfg.GDriveClientSecret, cfg.GDriveRefreshToken, cfg.GDriveFolder)
	}

	authSvc := service.NewAuthService(authRepo, gVerifier, cfg.JWTSecret, cfg.JWTExpiry)
	userSvc := service.NewUserService(userRepo, authRepo, houseRepo, photoRepo, payRepo, logRepo, settingsRepo)
	if lc := lark.New(cfg.LarkAPI, cfg.LarkAppID, cfg.LarkAppSecret, cfg.LarkCTVWiki, cfg.LarkCTVBase, cfg.LarkCTVTable); lc != nil {
		userSvc.SetLarkSource(lc)
	}
	// Hồ sơ Messenger từ dezon-inbox (chỉ đọc DB + kho ảnh S3)
	var inboxSrc inbox.Source
	if cfg.InboxDatabaseURL != "" {
		if inboxSrc, err = inbox.New(ctx, inbox.Config{DatabaseURL: cfg.InboxDatabaseURL, S3Endpoint: cfg.InboxS3Endpoint,
			S3Bucket: cfg.InboxS3Bucket, S3Access: cfg.InboxS3Access, S3Secret: cfg.InboxS3Secret, S3Region: cfg.InboxS3Region}); err != nil {
			log.Printf("dezon-inbox: %v (tắt đồng bộ Messenger)", err)
			inboxSrc = nil
		} else {
			defer inboxSrc.Close()
		}
	}
	geocoder := geo.New(cfg.GoogleMapsKey)
	// Khách hàng: pipeline Kinh doanh của Dezon Pro (khoá chỉ đọc). Không có khoá: tắt.
	var leads service.LeadSource
	if cfg.DezonDBURL != "" { // CSDL Dezon Pro, tài khoản chỉ đọc
		if d, err := dezontech.NewDB(context.Background(), cfg.DezonDBURL); err != nil {
			log.Printf("khách hàng: không mở được CSDL Dezon Pro: %v", err)
		} else {
			defer d.Close()
			leads = d
		}
	} else if c := dezontech.New(cfg.DezonTechURL, cfg.DezonTechKey); c != nil {
		leads = c
	}
	// Nhà thầu đăng ký qua form Lark + trích từ ảnh CTV (trang Nhà thầu)
	var ntSrc service.LarkCTVSource
	if lc := lark.New(cfg.LarkAPI, cfg.LarkAppID, cfg.LarkAppSecret, "", cfg.LarkNTBase, cfg.LarkNTTable); lc != nil {
		ntSrc = lc
	}
	ntSvc := service.NewContractorService(repository.NewContractorRepository(db), houseRepo, ntSrc)
	houseSvc := service.NewHouseService(service.HouseDeps{
		Houses: houseRepo, Photos: photoRepo, Logs: logRepo, Users: userRepo, Payments: payRepo, Settings: settingsRepo,
		Files: files, AI: ai.New(cfg.AIEnabled, cfg.AIModel, cfg.AIWorkspace), Geo: geocoder, OCR: ocr.New(),
		Drive: drive, Public: cfg.PublicURL, MediaKey: cfg.JWTSecret,
		Inbox: inboxSrc, Auth: authRepo, Sync: repository.NewSyncRepository(db), Customers: repository.NewCustomerRepository(db), Leads: leads,
	})
	paySvc := service.NewPaymentService(payRepo, houseRepo, photoRepo, userRepo, logRepo, settingsRepo, files)

	log.Printf("AI đọc ảnh: %v (%s) · Google Drive: %v · đăng nhập Google: %v", cfg.AIEnabled, cfg.AIModel, drive != nil, gVerifier.Enabled())
	log.Printf("Hồ sơ Messenger (dezon-inbox): %v, mỗi %s", inboxSrc != nil, cfg.InboxSyncEvery)
	go houseSvc.RunDriveSync() // đẩy các hồ sơ còn chờ từ lần chạy trước
	maskCtx, stopMask := context.WithCancel(context.Background())
	defer stopMask()
	go houseSvc.RunPhoneMaskScan(maskCtx) // che SĐT trên ảnh (ảnh cũ + ảnh mới)
	houseSvc.StartInboxSync(cfg.InboxSyncEvery)
	houseSvc.StartCustomerSync(15 * time.Minute)
	userSvc.StartLarkCTVSync(15 * time.Minute)
	ntSvc.StartLarkSync(15 * time.Minute)
	// Công trình nhà thầu mới đăng trên dezon.vn -> lớp ghim Dezon trên bản đồ (tra toạ độ: Google nếu có key)
	dezonRepo := repository.NewDezonRepository(db)
	log.Printf("Công trình Dezon: tra toạ độ bằng %s, mỗi 6h", map[bool]string{true: "Google", false: "OpenStreetMap"}[geocoder.Google()])
	service.NewDezonSync(dezonRepo, geocoder, dezon.ProjectsURL).Start(6 * time.Hour)
	go func() { // gán tỉnh mới (34 tỉnh) cho hồ sơ cũ
		if n, err := houseSvc.BackfillProvinces(context.Background()); err != nil {
			log.Printf("gán tỉnh: %v", err)
		} else if n > 0 {
			log.Printf("đã gán tỉnh cho %d hồ sơ", n)
		}
	}()

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: router.New(router.Deps{
			Config:         cfg,
			AuthHandler:    handler.NewAuthHandler(authSvc),
			UserHandler:    handler.NewUserHandler(userSvc, authSvc),
			HouseHandler:   handler.NewHouseHandler(houseSvc),
			PaymentHandler: handler.NewPaymentHandler(paySvc, gVerifier.ClientID()).WithNTForm(cfg.LarkNTForm),
			NT:             handler.NewNTHandler(service.NewNTService(repository.NewNTRepository(db), authRepo, files, logRepo, cfg.JWTSecret).WithGeo(geocoder)),
			Dezon:          handler.NewDezonHandler(dezon.ListURL, dezonRepo),
			Contractors:    handler.NewContractorHandler(ntSvc, cfg.LarkNTForm),
			UserLookup:     userRepo.GetByID,
			HealthChecks:   checks,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		// Tải tới 20 ảnh x 20MB qua 4G, và AI kiểm ảnh: cần thời gian dài
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("API listening on :%s (env=%s)", cfg.Port, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	houseSvc.Wait() // chờ AI / Drive đang chạy dở
	return err
}
