// ntctl: công cụ quản trị backend nghiệm thu.
//
//	go run ./cmd/ntctl migrate                                   chạy migrations/*.sql (không cần psql)
//	go run ./cmd/ntctl adduser <đăng nhập> "<Họ tên>" admin|ketoan <mật khẩu>
//	go run ./cmd/ntctl import-legacy -db /duong/dan/nghiemthu.db [-src thu_muc_chua_anh]
//
// Đọc DATABASE_URL, DATA_DIR từ môi trường hoặc file .env (như API).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"vuathau/backend/internal/database"
	"vuathau/backend/internal/legacy"
)

func usage() {
	fmt.Fprintln(os.Stderr, `Cách dùng:
  ntctl migrate [-dir migrations]
  ntctl adduser <đăng nhập> "<Họ tên>" admin|ketoan|sale <mật khẩu>
  ntctl import-legacy -db <nghiemthu.db> [-src <thư mục có anh/ và bienlai/>]
  ntctl reprice [-note "lý do"] <mã hồ sơ> ...     tính lại tiền hồ sơ chờ duyệt (ảnh đạt, loại chụp, video)
  ntctl attach -house <mã> [-note "lý do"] <sha256> ...   gắn lại ảnh có sẵn trong DATA_DIR/anh vào hồ sơ`)
	os.Exit(2)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	_ = godotenv.Load()
	if len(os.Args) < 2 {
		usage()
	}
	ctx := context.Background()
	pool, err := database.NewPostgres(ctx, env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/vuathau?sslmode=disable"))
	if err != nil {
		fail(err)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ExitOnError)
		dir := fs.String("dir", "migrations", "thư mục chứa file .sql")
		_ = fs.Parse(os.Args[2:])
		done, err := database.Migrate(ctx, pool, *dir)
		for _, f := range done {
			fmt.Println("==> đã chạy", f)
		}
		if err != nil {
			fail(err)
		}
		fmt.Printf("xong %d migration\n", len(done))
	case "adduser":
		if len(os.Args) != 6 {
			usage()
		}
		addUser(ctx, pool, os.Args[2], os.Args[3], os.Args[4], os.Args[5])
	case "reprice":
		fs := flag.NewFlagSet("reprice", flag.ExitOnError)
		note := fs.String("note", "", "lý do ghi vào lịch sử hồ sơ")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			usage()
		}
		reprice(ctx, pool, *note, fs.Args())
	case "attach":
		fs := flag.NewFlagSet("attach", flag.ExitOnError)
		house := fs.Int64("house", 0, "mã hồ sơ")
		note := fs.String("note", "", "lý do ghi vào lịch sử hồ sơ")
		_ = fs.Parse(os.Args[2:])
		if *house == 0 || fs.NArg() == 0 {
			usage()
		}
		attach(ctx, pool, env("DATA_DIR", "./data"), *house, *note, fs.Args())
	case "import-legacy":
		fs := flag.NewFlagSet("import-legacy", flag.ExitOnError)
		db := fs.String("db", "", "file SQLite của bản cũ (data/nghiemthu.db)")
		src := fs.String("src", "", "thư mục chứa anh/ và bienlai/ (mặc định: cạnh file .db)")
		_ = fs.Parse(os.Args[2:])
		if *db == "" {
			usage()
		}
		dataDir := env("DATA_DIR", "./data")
		st, err := legacy.Import(ctx, pool, legacy.Options{SQLitePath: *db, SourceDir: *src, DataDir: dataDir,
			Log: func(f string, a ...any) { fmt.Printf(f+"\n", a...) }})
		if err != nil {
			fail(err)
		}
		keys := make([]string, 0, len(st))
		for k := range st {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%-14s %d\n", k, st[k])
		}
		fmt.Println("Đã chuyển xong dữ liệu cũ. Ảnh đã chép vào", filepath.Join(dataDir, "anh"))
	default:
		usage()
	}
}

var (
	phoneRE = regexp.MustCompile(`^(0|\+84)\d{9,10}$`)
	loginRE = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)
)

func addUser(ctx context.Context, pool *pgxpool.Pool, login, name, role, pw string) {
	login = strings.ToLower(regexp.MustCompile(`[\s.\-]`).ReplaceAllString(login, ""))
	if role != "admin" && role != "ketoan" && role != "sale" {
		fail(fmt.Errorf("vai trò phải là admin, ketoan hoặc sale"))
	}
	if !phoneRE.MatchString(login) && !loginRE.MatchString(login) {
		fail(fmt.Errorf("tên đăng nhập: SĐT hoặc 3-30 ký tự a-z, 0-9, _"))
	}
	if len(pw) < 8 {
		fail(fmt.Errorf("mật khẩu từ 8 ký tự"))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		fail(err)
	}
	// Đã có thì đổi mật khẩu, vai trò, tên (và mở khoá): dùng để lấy lại quyền admin
	_, err = pool.Exec(ctx, `INSERT INTO users (username, name, role, password_hash) VALUES ($1, $2, $3, $4)
		ON CONFLICT (username) DO UPDATE SET name = EXCLUDED.name, role = EXCLUDED.role, password_hash = EXCLUDED.password_hash,
			locked = 0, token_version = users.token_version + 1`, login, strings.TrimSpace(name), role, string(hash))
	if err != nil {
		fail(err)
	}
	fmt.Printf("Đã tạo / cập nhật %s %s (%s)\n", role, name, login)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lỗi:", err)
	os.Exit(1)
}
