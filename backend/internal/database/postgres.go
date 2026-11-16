package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgres creates a connection pool and verifies the connection.
func NewPostgres(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
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
