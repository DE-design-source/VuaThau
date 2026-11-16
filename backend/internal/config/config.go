package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, read from environment variables.
type Config struct {
	AppEnv string
	Port   string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	JWTSecret string
	JWTExpiry time.Duration

	CORSAllowedOrigins []string

	RateLimitRPS   float64
	RateLimitBurst int

	// ---- Nghiệm thu ----

	// DataDir chứa ảnh hồ sơ (anh/) và ảnh biên lai (bienlai/). SAO LƯU thư mục này cùng Postgres.
	DataDir string
	// PublicURL: địa chỉ web quản lý, ghi vào file thông tin trên Google Drive.
	PublicURL string
	// StaticDir: thư mục build của web (frontend/dist). Đặt thì API phục vụ luôn web ở "/".
	StaticDir string

	// AI đọc ảnh (Claude). Bật khi có ANTHROPIC_API_KEY hoặc ANTHROPIC_AUTH_TOKEN (SDK tự đọc).
	AIEnabled   bool
	AIModel     string
	AIWorkspace string // ANTHROPIC_WORKSPACE_ID, chỉ cần với key cá nhân (sk-ant-usr-...)

	// Tra địa chỉ từ GPS: có key Google thì dùng Google Geocoding, không thì OpenStreetMap.
	GoogleMapsKey string

	// Đăng nhập / đăng ký bằng Google trong app: OAuth client loại Web (id đầu tiên gửi cho app).
	GoogleClientIDs []string

	// Google Drive: mỗi hồ sơ gửi duyệt thành 1 thư mục con. Thiếu refresh token: tắt.
	GDriveClientID     string
	GDriveClientSecret string
	GDriveRefreshToken string
	GDriveFolder       string

	// Hồ sơ CTV gửi qua Messenger (hệ thống dezon-inbox). Trống INBOX_DATABASE_URL: tắt.
	InboxDatabaseURL string
	InboxS3Endpoint  string
	InboxS3Bucket    string
	InboxS3Access    string
	InboxS3Secret    string
	InboxS3Region    string
	InboxSyncEvery   time.Duration
}

// Load reads configuration from the environment. A .env file in the working
// directory is loaded first if present; real environment variables win.
func Load() (*Config, error) {
	_ = godotenv.Load()
	var err error

	cfg := &Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/vuathau?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),

		DataDir:            getEnv("DATA_DIR", "./data"),
		PublicURL:          strings.TrimRight(getEnv("PUBLIC_URL", "https://nghiemthu.vuathau.com.vn"), "/"),
		StaticDir:          getEnv("STATIC_DIR", ""),
		AIEnabled:          os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "",
		AIModel:            getEnv("AI_MODEL", "claude-opus-5-5"),
		AIWorkspace:        getEnv("ANTHROPIC_WORKSPACE_ID", ""),
		GoogleMapsKey:      getEnv("GOOGLE_MAPS_API_KEY", ""),
		GoogleClientIDs:    splitCSV(getEnv("GOOGLE_CLIENT_IDS", "")),
		GDriveClientID:     getEnv("GDRIVE_CLIENT_ID", ""),
		GDriveClientSecret: getEnv("GDRIVE_CLIENT_SECRET", ""),
		GDriveRefreshToken: getEnv("GDRIVE_REFRESH_TOKEN", ""),
		GDriveFolder:       getEnv("GDRIVE_FOLDER", ""),
		InboxDatabaseURL:   strings.TrimPrefix(strings.TrimSpace(getEnv("INBOX_DATABASE_URL", "")), "﻿"),
		InboxS3Endpoint:    getEnv("INBOX_S3_ENDPOINT", ""),
		InboxS3Bucket:      getEnv("INBOX_S3_BUCKET", ""),
		InboxS3Access:      getEnv("INBOX_S3_ACCESS_KEY", ""),
		InboxS3Secret:      getEnv("INBOX_S3_SECRET_KEY", ""),
		InboxS3Region:      getEnv("INBOX_S3_REGION", "us-east-1"),
	}
	if cfg.InboxSyncEvery, err = time.ParseDuration(getEnv("INBOX_SYNC_EVERY", "5m")); err != nil {
		return nil, fmt.Errorf("INBOX_SYNC_EVERY: %w", err)
	}

	if cfg.RedisDB, err = strconv.Atoi(getEnv("REDIS_DB", "0")); err != nil {
		return nil, fmt.Errorf("REDIS_DB: %w", err)
	}
	// App điện thoại giữ đăng nhập lâu: mặc định 30 ngày
	if cfg.JWTExpiry, err = time.ParseDuration(getEnv("JWT_EXPIRY", "720h")); err != nil {
		return nil, fmt.Errorf("JWT_EXPIRY: %w", err)
	}
	if cfg.RateLimitRPS, err = strconv.ParseFloat(getEnv("RATE_LIMIT_RPS", "10"), 64); err != nil {
		return nil, fmt.Errorf("RATE_LIMIT_RPS: %w", err)
	}
	if cfg.RateLimitBurst, err = strconv.Atoi(getEnv("RATE_LIMIT_BURST", "40")); err != nil {
		return nil, fmt.Errorf("RATE_LIMIT_BURST: %w", err)
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, errors.New("JWT_SECRET is required and must be at least 32 characters")
	}
	if cfg.JWTExpiry <= 0 {
		return nil, errors.New("JWT_EXPIRY must be positive")
	}
	if cfg.RateLimitRPS <= 0 || cfg.RateLimitBurst <= 0 {
		return nil, errors.New("RATE_LIMIT_RPS and RATE_LIMIT_BURST must be positive")
	}

	return cfg, nil
}

func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

// DriveEnabled: đủ thông tin để đẩy hồ sơ lên Google Drive.
func (c *Config) DriveEnabled() bool {
	return c.GDriveClientID != "" && c.GDriveClientSecret != "" && c.GDriveRefreshToken != "" && c.GDriveFolder != ""
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
