package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate chạy mọi file .sql trong dir theo thứ tự tên (giống scripts/migrate.sh, không cần psql).
// Các migration viết idempotent nên chạy lại an toàn. Trả tên các file đã chạy.
func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("không thấy migration nào trong %s", dir)
	}
	sort.Strings(files)
	done := make([]string, 0, len(files))
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return done, err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return done, err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return done, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return done, err
		}
		done = append(done, filepath.Base(f))
	}
	return done, nil
}
