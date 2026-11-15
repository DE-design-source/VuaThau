# vuathau backend

Backend REST API + WebSocket viết bằng Go (module `vuathau/backend`): đăng ký/đăng nhập bằng JWT,
quản lý người dùng và nhắn tin trực tiếp giữa hai người dùng, đẩy tin nhắn mới theo thời gian thực
qua WebSocket. Lưu trữ bằng PostgreSQL, kiểm tra sức khỏe kèm Redis.

- Chỉ dùng thư viện chuẩn `net/http` (router `ServeMux` của Go 1.22+), `pgx`, `go-redis`, `gorilla/websocket`, `golang-jwt`.
- Tài liệu API chi tiết: [docs/api.md](docs/api.md).

## Cấu trúc thư mục

```
backend/
├── cmd/
│   └── api/main.go            # Điểm khởi chạy: đọc config, kết nối DB/Redis, ghép các tầng, chạy HTTP server, tắt êm (graceful shutdown)
├── internal/
│   ├── config/                # Đọc biến môi trường (+ file .env) thành struct Config
│   ├── database/              # Tạo pool PostgreSQL (pgx) và client Redis, ping khi khởi động
│   ├── model/                 # Struct dữ liệu dùng chung (User, Message) kèm tag JSON
│   ├── repository/            # Truy cập DB: câu SQL, map lỗi Postgres -> ErrNotFound/ErrDuplicate
│   ├── service/               # Nghiệp vụ: validate input, hash mật khẩu, phát JWT, phân trang, gửi sự kiện realtime
│   ├── handler/               # HTTP handler: parse request, gọi service, map lỗi service -> mã HTTP (helpers.go)
│   ├── middleware/            # Auth (JWT), CORS, rate limit theo IP
│   ├── router/                # Khai báo route, gắn middleware, endpoint /health
│   ├── websocket/             # Hub quản lý kết nối WebSocket theo user, đẩy sự kiện tới client
│   └── pkg/
│       ├── response/          # Envelope JSON chuẩn {success, data?, error?}
│       └── token/             # Tạo/kiểm tra JWT HS256
├── migrations/                # File SQL tạo bảng (chạy theo thứ tự tên file)
├── docs/                      # Tài liệu (api.md)
├── scripts/                   # Script tiện ích: migrate.sh (chạy migration), backup.sh (pg_dump)
├── tests/                     # Test (go test)
├── deployments/               # docker-compose.yml (postgres, redis, api)
├── Dockerfile                 # Build image cho API
├── .env.example               # Mẫu biến môi trường
└── go.mod
```

## Luồng xử lý request

```
Client
  -> router            (khớp method + path)
  -> middleware        (CORS -> RateLimit -> Auth nếu route cần đăng nhập)
  -> handler           (decode JSON, lấy user id từ context, gọi service)
  -> service           (validate, nghiệp vụ, trả lỗi sentinel: ErrInvalidInput, ErrNotFound, ...)
  -> repository        (SQL qua pgx)
  -> PostgreSQL
```

Kết quả trả ngược lại theo chiều ngược; handler bọc dữ liệu trong envelope
`{"success": true, "data": ...}` hoặc `{"success": false, "error": "..."}`.
Khi gửi tin nhắn, service còn đẩy sự kiện `message:new` qua WebSocket Hub.

## Chạy nhanh

Yêu cầu: Go (theo phiên bản trong `go.mod`), Docker + Docker Compose. Các lệnh chạy trong thư mục `backend/`.

Tạo file cấu hình (bắt buộc có `JWT_SECRET` dài **tối thiểu 32 ký tự**, nếu không API sẽ không khởi động):

```bash
cp .env.example .env   # rồi thay JWT_SECRET, ví dụ: openssl rand -base64 48
```

Các biến môi trường: `APP_ENV`, `PORT`, `DATABASE_URL`, `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`,
`JWT_SECRET`, `JWT_EXPIRY`, `CORS_ALLOWED_ORIGINS`, `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`
(xem giải thích và giá trị mặc định trong `.env.example`). `JWT_EXPIRY`, `RATE_LIMIT_RPS`,
`RATE_LIMIT_BURST` phải lớn hơn 0.

### a) Chạy API trên máy, Postgres + Redis bằng Docker

```bash
docker compose -f deployments/docker-compose.yml up -d postgres redis
go run ./cmd/api
```

API lắng nghe tại `http://localhost:8080`. Kiểm tra: `curl http://localhost:8080/health`.

### b) Chạy toàn bộ bằng Docker

```bash
docker compose -f deployments/docker-compose.yml up --build
```

Service `api` đọc cấu hình (gồm `JWT_SECRET`) từ `backend/.env`, nên cần tạo `.env` trước.
Compose chỉ ghi đè `PORT`, `DATABASE_URL`, `REDIS_ADDR` để trỏ tới các container `postgres`/`redis`.

### Chạy test

```bash
go test ./...
```

### Migration

- Container Postgres mount `migrations/` vào `/docker-entrypoint-initdb.d`, nên các file `.sql`
  **tự chạy một lần khi khởi tạo volume dữ liệu rỗng lần đầu**. Thêm migration mới sau đó sẽ
  không tự chạy lại; dùng script hoặc xóa volume (`docker compose -f deployments/docker-compose.yml down -v`, mất dữ liệu).
- Chạy thủ công (cần `psql`; đọc `DATABASE_URL` từ môi trường hoặc `.env`):

  ```bash
  scripts/migrate.sh
  ```

## Thêm một tính năng mới

Ví dụ thêm tài nguyên `Post`:

1. **Model** – `internal/model/post.go`: struct `Post` với tag JSON.
2. **Migration** – `migrations/00X_create_posts.sql`: tạo bảng (viết idempotent, ví dụ `CREATE TABLE IF NOT EXISTS`).
3. **Repository** – `internal/repository/post_repository.go`: interface `PostRepository` + implementation pgx; map lỗi về `ErrNotFound`/`ErrDuplicate`.
4. **Service** – `internal/service/post_service.go`: interface `PostService`, validate input, trả lỗi bọc `ErrInvalidInput`/`ErrNotFound`/`ErrConflict` để handler map đúng mã HTTP.
5. **Handler** – `internal/handler/post_handler.go`: dùng `decodeJSON`, `writeServiceError`, `response.Success`.
6. **Route** – thêm vào `internal/router/router.go` (`mux.Handle("POST /api/v1/posts", protected(...))`), thêm field vào `router.Deps` và khởi tạo repository -> service -> handler trong `cmd/api/main.go`.
7. Viết test trong `tests/` và cập nhật `docs/api.md`.
