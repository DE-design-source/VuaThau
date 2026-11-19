package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgres creates a connection pool and verifies the connection.
func NewPostgres(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse postgres url: %w", err)
	}
	// Mặc định pgx chỉ mở max(4, số CPU) kết nối: nhiều CTV cùng tải ảnh / mở app thì phải xếp hàng chờ.
	// 20 (Postgres cho tối đa 40); đặt pool_max_conns trong DATABASE_URL để đổi.
	if !strings.Contains(url, "pool_max_conns") {
		cfg.MaxConns = 20
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	// Dữ liệu tiếng Việt: CSDL phải mã hoá UTF-8 (Postgres trên Windows hay mặc định WIN1252)
	var enc string
	if err := pool.QueryRow(pingCtx, `SHOW server_encoding`).Scan(&enc); err == nil && enc != "UTF8" {
		pool.Close()
		return nil, fmt.Errorf("CSDL đang mã hoá %s, cần UTF8: tạo lại bằng CREATE DATABASE ... ENCODING 'UTF8' TEMPLATE template0", enc)
	}

	return pool, nil
}
