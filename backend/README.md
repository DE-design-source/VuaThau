# vuathau backend

Backend REST API viết bằng Go (module `vuathau/backend`) cho **Vua Thầu Nghiệm thu**: cộng tác viên (CTV) chụp ảnh công trình
đang xây trên app, AI kiểm ảnh, admin duyệt, kế toán thanh toán. Một API `/api/v1` dùng chung cho:

- **app Android của CTV** (Flutter, repo `app-Vua-Thau-`)
- **web quản lý của admin và kế toán** (React, `../frontend`)

nên app và web chung dữ liệu. Ngoài ra vẫn giữ phần khung ban đầu: nhắn tin + WebSocket.

- Chỉ dùng `net/http` (ServeMux Go 1.22+), `pgx`, `golang-jwt`, `gorilla/websocket`; AI qua SDK chính thức `anthropic-sdk-go`.
- Đặc tả API nghiệm thu: [docs/nghiemthu-api.md](docs/nghiemthu-api.md). Phần khung: [docs/api.md](docs/api.md).

## Cấu trúc thư mục

```
backend/
├── cmd/
│   ├── api/main.go            # API: đọc config, kết nối DB, ghép repository -> service -> handler, chạy HTTP, tắt êm
│   ├── ntctl/                 # Quản trị: migrate, adduser (admin / kế toán), import-legacy (chuyển dữ liệu bản Python cũ)
│   └── devdb/                 # Postgres nhúng cho máy dev (không cần Docker)
├── internal/
│   ├── config/                # Biến môi trường (+ .env)
│   ├── database/              # Pool Postgres (bắt buộc UTF8), Redis, chạy migration
│   ├── model/                 # User, House, Photo, Batch, Adjustment, Settings...
│   ├── repository/            # SQL qua pgx; trả tiền / huỷ phiếu chạy trong giao dịch có khoá dòng
│   ├── service/               # Nghiệp vụ: luật kiểm ảnh, tiền, duyệt, thanh toán, báo cáo (rules.go là luật thuần, có test)
│   ├── handler/               # HTTP: parse request, gọi service, map lỗi -> mã HTTP
│   ├── middleware/            # Auth (JWT), Session (nạp người dùng, chặn token cũ / tài khoản khoá, phân quyền), CORS, rate limit
│   ├── router/                # Route + vai trò được gọi; phục vụ web React nếu đặt STATIC_DIR
│   ├── integration/           # ai (Claude đọc ảnh), geo (địa chỉ từ GPS), google (xác minh Google Sign-In), gdrive (lưu hồ sơ)
│   ├── storage/               # File ảnh hồ sơ + biên lai trong DATA_DIR
│   ├── legacy/                # Chuyển SQLite + ảnh của bản Python sang Postgres
│   ├── pkg/                   # response (envelope), token (JWT), imaging (EXIF, xoay, thu nhỏ, dHash), xlsx
│   └── websocket/             # Hub WebSocket (khung ban đầu)
├── migrations/                # 001 users, 002 messages, 003 nghiệm thu
├── docs/                      # api.md, nghiemthu-api.md
├── tests/                     # Test tích hợp trên Postgres thật (tự chạy Postgres nhúng)
└── deployments/               # docker-compose.yml
```

## Luồng nghiệp vụ

```
CTV (app)  -> POST /houses (ảnh + giờ chụp/GPS)  -> luật kiểm ảnh + AI đọc chữ Timemark, bảng nhà thầu (chạy nền)
           -> PUT /houses/{id} submit           -> chờ duyệt (tính tiền theo số ảnh đạt, cảnh báo nhà gần nhau < 30m)
Admin (web)-> POST /houses/{id}/review          -> đã duyệt / trả lại / từ chối
Kế toán    -> POST /payouts/{ctv} hoặc /accounting/pay -> phiếu chi (đủ số hồ sơ đã duyệt, đối chiếu đúng danh sách đang thấy)
CTV (app)  -> GET /ctv/income, /batches/{id}    -> thấy tiền, mã giao dịch, biên lai
```

## Chạy trên máy dev

Cần Go (bản trong `go.mod`). Không cần Docker, Redis là tuỳ chọn (chỉ dùng cho `/health`).

```bash
cp .env.example .env          # đặt JWT_SECRET (>= 32 ký tự)
go run ./cmd/devdb            # cửa sổ 1: Postgres nhúng ở cổng 5432
go run ./cmd/ntctl migrate    # tạo bảng
go run ./cmd/ntctl adduser admin "Quản trị" admin <mật-khẩu-8-ký-tự>
go run ./cmd/api              # API ở http://localhost:8080
```

Web: `cd ../frontend && npm run dev` (Vite chuyển `/api` sang :8080). App: `flutter run --dart-define=API_URL=http://10.0.2.2:8080`.

## Test

```bash
go test ./...
```

Test tích hợp tự tải và chạy Postgres nhúng (hoặc dùng `TEST_DATABASE_URL` nếu đặt), mỗi test 1 CSDL riêng. Có luồng
đầu-cuối CTV nộp -> admin duyệt -> kế toán trả -> huỷ phiếu, phân quyền, đổi mật khẩu / khoá, và chuyển dữ liệu bản cũ.

## Chuyển dữ liệu từ bản Python cũ

```bash
# DATABASE_URL trỏ tới CSDL mới (đã migrate, còn trống), DATA_DIR là thư mục ảnh của backend Go
go run ./cmd/ntctl import-legacy -db /duong/dan/data/nghiemthu.db
```

Giữ nguyên id hồ sơ, phiếu chi; chép ảnh, biên lai; mật khẩu cũ (scrypt) vẫn đăng nhập được và tự nâng lên bcrypt.

## Biến môi trường

Xem `.env.example`. Bắt buộc: `JWT_SECRET`, `DATABASE_URL` (CSDL **UTF8**). Nghiệm thu: `DATA_DIR`, `STATIC_DIR`,
`ANTHROPIC_API_KEY` (bật AI), `GOOGLE_MAPS_API_KEY`, `GOOGLE_CLIENT_IDS`, `GDRIVE_*`.

## Thêm một tính năng mới

1. **Model** – `internal/model/...`
2. **Migration** – `migrations/00X_....sql` (idempotent)
3. **Repository** – interface + pgx, map lỗi về `ErrNotFound` / `ErrDuplicate`
4. **Service** – validate, trả lỗi `fail(ErrInvalidInput, "câu báo")` để handler trả đúng mã HTTP và câu báo tiếng Việt
5. **Handler** – `bodyOr400`, `writeServiceError`, `ok` / `msg`
6. **Route** – `internal/router/router.go`, `role(handler, vai trò...)`
7. Test trong `tests/`, cập nhật `docs/nghiemthu-api.md`
