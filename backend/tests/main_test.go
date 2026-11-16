package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Test chạy trên PostgreSQL thật:
//   - TEST_DATABASE_URL đặt sẵn (vd CI có Postgres): dùng máy chủ đó.
//   - không đặt: tự tải và chạy Postgres nhúng (embedded-postgres) ở cổng 54329, không cần cài đặt.
// CSDL "vt_template" chạy mọi migration; mỗi test tạo 1 CSDL riêng sao từ mẫu nên chạy song song được.

var (
	adminURL string // kết nối tới CSDL "postgres" để tạo / xoá CSDL test
	baseURL  func(db string) string
	dbSeq    atomic.Int64
	createMu sync.Mutex
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	var stop func() error
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		baseURL = func(db string) string { return replaceDB(url, db) }
	} else {
		dir, err := os.MkdirTemp("", "vt-pg-*")
		if err != nil {
			fmt.Println("tạo thư mục tạm:", err)
			return 1
		}
		defer os.RemoveAll(dir)
		pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
			Port(54329).Username("postgres").Password("postgres").Database("postgres").
			Encoding("UTF8").Locale("C").
			RuntimePath(filepath.Join(dir, "run")).DataPath(filepath.Join(dir, "data")).
			BinariesPath(filepath.Join(os.TempDir(), "vt-pg-bin")).Logger(nil))
		if err := pg.Start(); err != nil {
			fmt.Println("chạy Postgres nhúng:", err)
			return 1
		}
		stop = pg.Stop
		baseURL = func(db string) string {
			return fmt.Sprintf("postgres://postgres:postgres@localhost:54329/%s?sslmode=disable", db)
		}
	}
	if stop != nil {
		defer func() { _ = stop() }()
	}
	adminURL = baseURL("postgres")
	if err := makeTemplate(ctx); err != nil {
		fmt.Println("tạo CSDL mẫu:", err)
		return 1
	}
	return m.Run()
}

func replaceDB(url, db string) string {
	// postgres://u:p@h:port/dbname?query -> thay dbname
	q := ""
	if i := indexOf(url, '?'); i >= 0 {
		url, q = url[:i], url[i:]
	}
	if i := lastIndexOf(url, '/'); i > len("postgres://") {
		url = url[:i]
	}
	return url + "/" + db + q
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func lastIndexOf(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func makeTemplate(ctx context.Context) error {
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return err
	}
	defer admin.Close()
	_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS vt_template`)
	if _, err := admin.Exec(ctx, `CREATE DATABASE vt_template`); err != nil {
		return err
	}
	tpl, err := pgxpool.New(ctx, baseURL("vt_template"))
	if err != nil {
		return err
	}
	defer tpl.Close()
	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := tpl.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	// chạy lại lần 2: migration phải idempotent
	for _, f := range files {
		sql, _ := os.ReadFile(f)
		if _, err := tpl.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s (lần 2): %w", filepath.Base(f), err)
		}
	}
	return nil
}

// newDB tạo 1 CSDL riêng cho test từ mẫu, tự xoá khi test xong.
func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	name := fmt.Sprintf("vt_test_%d", dbSeq.Add(1))
	createMu.Lock() // Postgres không cho 2 lệnh CREATE DATABASE cùng sao 1 mẫu một lúc
	defer createMu.Unlock()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("kết nối Postgres: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s TEMPLATE vt_template`, name)); err != nil {
		t.Fatalf("tạo CSDL test: %v", err)
	}
	pool, err := pgxpool.New(ctx, baseURL(name))
	if err != nil {
		t.Fatalf("kết nối CSDL test: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if a, err := pgxpool.New(context.Background(), adminURL); err == nil {
			_, _ = a.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, name))
			a.Close()
		}
	})
	return pool
}
