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
}

// Load reads configuration from the environment. A .env file in the working
// directory is loaded first if present; real environment variables win.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/vuathau?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
	}

	var err error
	if cfg.RedisDB, err = strconv.Atoi(getEnv("REDIS_DB", "0")); err != nil {
		return nil, fmt.Errorf("REDIS_DB: %w", err)
	}
	if cfg.JWTExpiry, err = time.ParseDuration(getEnv("JWT_EXPIRY", "24h")); err != nil {
		return nil, fmt.Errorf("JWT_EXPIRY: %w", err)
	}
	if cfg.RateLimitRPS, err = strconv.ParseFloat(getEnv("RATE_LIMIT_RPS", "10"), 64); err != nil {
		return nil, fmt.Errorf("RATE_LIMIT_RPS: %w", err)
	}
	if cfg.RateLimitBurst, err = strconv.Atoi(getEnv("RATE_LIMIT_BURST", "20")); err != nil {
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
