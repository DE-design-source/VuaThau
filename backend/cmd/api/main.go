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
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/router"
	"vuathau/backend/internal/service"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Infrastructure
	db, err := database.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	rdb, err := database.NewRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer rdb.Close()

	hub := websocket.NewHub(cfg.CORSAllowedOrigins)
	defer hub.Close()

	// Repository -> Service -> Handler
	authSvc := service.NewAuthService(repository.NewAuthRepository(db), cfg.JWTSecret, cfg.JWTExpiry)
	userSvc := service.NewUserService(repository.NewUserRepository(db))
	msgSvc := service.NewMessageService(repository.NewMessageRepository(db), hub)

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: router.New(router.Deps{
			Config:         cfg,
			AuthHandler:    handler.NewAuthHandler(authSvc),
			UserHandler:    handler.NewUserHandler(userSvc),
			MessageHandler: handler.NewMessageHandler(msgSvc),
			Hub:            hub,
			HealthChecks: map[string]router.HealthCheck{
				"postgres": db.Ping,
				"redis":    func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
			},
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
