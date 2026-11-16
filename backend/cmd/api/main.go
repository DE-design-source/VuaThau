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
	"vuathau/backend/internal/integration/gdrive"
	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/integration/google"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/router"
	"vuathau/backend/internal/service"
	"vuathau/backend/internal/storage"
	"vuathau/backend/internal/websocket"
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

	hub := websocket.NewHub(cfg.CORSAllowedOrigins)
	defer hub.Close()

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
	msgSvc := service.NewMessageService(repository.NewMessageRepository(db), hub)
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
	houseSvc := service.NewHouseService(service.HouseDeps{
		Houses: houseRepo, Photos: photoRepo, Logs: logRepo, Users: userRepo, Payments: payRepo, Settings: settingsRepo,
		Files: files, AI: ai.New(cfg.AIEnabled, cfg.AIModel, cfg.AIWorkspace), Geo: geo.New(cfg.GoogleMapsKey),
		Drive: drive, Public: cfg.PublicURL,
		Inbox: inboxSrc, Auth: authRepo, Sync: repository.NewSyncRepository(db),
	})
	paySvc := service.NewPaymentService(payRepo, houseRepo, photoRepo, userRepo, logRepo, settingsRepo, files)

	log.Printf("AI đọc ảnh: %v (%s) · Google Drive: %v · đăng nhập Google: %v", cfg.AIEnabled, cfg.AIModel, drive != nil, gVerifier.Enabled())
	log.Printf("Hồ sơ Messenger (dezon-inbox): %v, mỗi %s", inboxSrc != nil, cfg.InboxSyncEvery)
	go houseSvc.RunDriveSync() // đẩy các hồ sơ còn chờ từ lần chạy trước
	houseSvc.StartInboxSync(cfg.InboxSyncEvery)

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: router.New(router.Deps{
			Config:         cfg,
			AuthHandler:    handler.NewAuthHandler(authSvc),
			UserHandler:    handler.NewUserHandler(userSvc, authSvc),
			MessageHandler: handler.NewMessageHandler(msgSvc),
			HouseHandler:   handler.NewHouseHandler(houseSvc),
			PaymentHandler: handler.NewPaymentHandler(paySvc, gVerifier.ClientID()),
			UserLookup:     userRepo.GetByID,
			Hub:            hub,
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
