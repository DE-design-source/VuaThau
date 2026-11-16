# API nghiệm thu (`/api/v1`)

Dùng chung cho **app CTV** (Flutter) và **web quản lý** (React, admin + kế toán). Cùng một CSDL nên dữ liệu thông nhau.

Quy ước chung (như [api.md](api.md)):

- Mọi response là envelope `{"success": true, "data": ...}` hoặc `{"success": false, "error": "câu báo tiếng Việt"}`.
- Đăng nhập trả JWT; gửi `Authorization: Bearer <token>`. Đổi mật khẩu, bị khoá: token cũ hết hiệu lực (401).
- Mã lỗi: 400 dữ liệu sai, 401 chưa đăng nhập / hết phiên, 403 không có quyền hoặc tài khoản bị khoá, 404 không thấy, 409 trùng.
- Thời gian (`*_at`) là RFC 3339 giờ Việt Nam, vd `2026-11-13T14:13:05+07:00`. `exif_time`, `ai_time` của ảnh là chuỗi `YYYY-MM-DD HH:MM`.
- Tiền là số nguyên đồng (VND).
- Thao tác ghi trả `data.message` (câu báo để hiện cho người dùng) cùng các trường riêng.
- Vai trò: `ctv` (cộng tác viên), `admin`, `ketoan` (kế toán). Cột "Ai" ghi vai trò được gọi.

Trạng thái hồ sơ: `nhap` (đang soạn), `tra_lai` (trả lại, CTV sửa), `cho_duyet`, `tu_choi`, `da_duyet` (chờ thanh toán), `da_chi` (đã thanh toán).

## Đối tượng

```jsonc
// User
{ "id": 7, "username": "0901234567", "name": "Nguyễn Văn A", "role": "ctv", "email": "a@gmail.com",
  "bank": "Vietcombank · 0123456789 · NGUYEN VAN A", "locked": 0, "created_at": "..." }
// locked: 0 bình thường, 1 bị khoá, 2 CTV đã tự xoá tài khoản

// Bank (tách từ User.bank)
{ "name": "Vietcombank", "no": "0123456789", "owner": "NGUYEN VAN A" }

// Money (tiền của 1 CTV, đã cộng thưởng/trừ)
{ "paid": 350000, "owed": 175000, "approved": 5, "reviewing": 35000 }

// HouseRow (dòng trong danh sách)
{ "id": 12, "ctv_id": 7, "ctv_name": "Nguyễn Văn A", "status": "cho_duyet", "address": "...", "amount": 35000,
  "dup": "", "created_at": "...", "cover_id": 101, "photos_total": 6, "photos_valid": 5 }

// House (chi tiết) = HouseRow + các trường dưới
{ "ctv_phone": "0901234567", "ctv_bank": "...", "lat": 10.8, "lng": 106.6, "contractor_name": "...",
  "contractor_phone": "...", "floors": 3, "stage": "Xây phần thô", "note": "lý do trả lại / từ chối",
  "ai_state": "", "batch_id": null }
// ai_state: "" chưa đọc, "dang_doc" AI đang đọc, "xong", "loi"

// Photo
{ "id": 101, "exif_time": "2026-11-13 14:10", "ai_time": "", "lat": 10.8, "lng": 106.6, "address": "...",
  "is_site": true, "dup_of": null, "problems": ["Ảnh chụp quá 30 ngày"] }   // problems rỗng = ảnh đạt

// Log
{ "id": 1, "house_id": 12, "batch_id": null, "user_name": "Admin", "action": "Duyệt", "note": "", "at": "..." }

// Step (tiến trình 4 bước của hồ sơ)
{ "title": "Admin duyệt", "who": "Admin", "state": "done|current|todo|bad", "at": "...|''", "hint": "" }

// Batch (phiếu chi)
{ "id": 3, "status": "da_chi|huy", "total": 210000, "ref": "FT123", "has_receipt": true, "receipt_version": "a1b2",
  "ctv_id": 7, "ctv_name": "...", "ctv_phone": "...", "ctv_bank": "...", "by_name": "Kế toán A",
  "created_at": "...", "house_count": 6 }

// Adjustment (thưởng > 0, trừ < 0)
{ "id": 2, "ctv_id": 7, "amount": -20000, "note": "...", "batch_id": null, "by_name": "...", "created_at": "..." }
```

## Công khai

| Method | Path | Mô tả |
|---|---|---|
| GET | `/config` | Hằng số chung (xem dưới) |
| POST | `/auth/login` | `{login, password}` → `AuthResult` |
| POST | `/auth/register` | CTV đăng ký → `AuthResult` |
| POST | `/auth/google` | `{id_token}` → `AuthResult` hoặc `{registered:false, google:{email,name}}` |

`GET /config`:
```jsonc
{ "statuses": { "nhap": {"label": "Đang soạn", "ctv_label": "Chưa gửi", "tone": "draft"}, ... },
  "status_order": ["nhap","tra_lai","cho_duyet","tu_choi","da_duyet","da_chi"],
  "min_photos": 5, "max_photos": 20, "pay_min": 5,
  "pay": [0,0,0,0,0,35000,50000, ...],          // pay[n]: tiền 1 hồ sơ có n ảnh đạt (n = 0..max_photos)
  "stages": ["Đào móng / làm móng", ...], "banks": ["Vietcombank", ...],
  "reasons": {"tra_lai": [...], "tu_choi": [...]},
  "google_client_id": "" }                         // trống: ẩn nút Google
```

`AuthResult`: `{ "user": User, "access_token": "...", "expires_in": 2592000 }`.

`POST /auth/login`: `login` là SĐT (CTV) hoặc tên đăng nhập (nhân viên). Sai 5 lần liền: khoá đăng nhập 5 phút (429).
Tài khoản bị khoá: 403.

`POST /auth/register` (CTV):
`{ "name", "phone", "password", "bank_name", "bank_no", "bank_owner", "id_token"? }`.
Có `id_token` (Google): gắn Google vào tài khoản mới. Nếu SĐT đã có tài khoản CTV chưa gắn Google và đúng mật khẩu:
gắn Google vào tài khoản cũ (không cần ngân hàng). SĐT trùng mà sai mật khẩu: 409.

## Tài khoản (mọi vai trò)

| Method | Path | Mô tả |
|---|---|---|
| GET | `/me` | `{user, bank: Bank, money?: Money, todo: n}` (money chỉ với CTV; todo = số việc chờ của vai trò) |
| PUT | `/me/bank` | CTV: `{bank_name, bank_no, bank_owner}` → `{message, user}` |
| PUT | `/me/password` | `{old, new}` → `{message, access_token, expires_in}` (token mới) |
| DELETE | `/me` | CTV tự xoá tài khoản → `{message}` |

`todo`: CTV = hồ sơ nháp + bị trả lại; admin = hồ sơ chờ duyệt; kế toán = số CTV đủ điều kiện thanh toán.

## CTV

| Method | Path | Mô tả |
|---|---|---|
| GET | `/ctv/home?tab=` | Trang Hồ sơ của tôi |
| GET | `/ctv/income` | Thu nhập |
| GET | `/geocode?lat=&lng=` | `{address}` (lỗi tra cứu: `""`) |
| POST | `/photos/check` | multipart `file`, `meta`? → `{problems: [], ai: bool}` (AI kiểm 1 ảnh trước khi nộp) |
| POST | `/houses` | multipart `files` (nhiều), `meta`? → `{message, messages: [], house_id: 12 \| null}` |
| PUT | `/houses/{id}` | `{address, floors, stage, contractor_name, contractor_phone, submit: bool}` → `{message, submitted}` |
| POST | `/houses/{id}/photos` | multipart `files`, `meta`? → `{message, messages, added}` |
| DELETE | `/houses/{id}/photos/{pid}` | → `{message}` |
| GET | `/houses/{id}/state` | `{ai_state, status}` (hỏi định kỳ khi AI đang đọc) |

`meta`: JSON mảng (cùng thứ tự `files`) hoặc object (với `/photos/check`) do camera trong app gửi:
`{"t": "YYYY-MM-DD HH:MM", "lat": 10.8, "lng": 106.6, "acc": 8, "addr": "..."}`. Chỉ dùng khi ảnh không có EXIF tương ứng.

`GET /ctv/home?tab=`: tab là `""`, `can_lam`, `dang_duyet`, `da_duyet`, `da_nhan`, `tu_choi`.
```jsonc
{ "rows": [HouseRow], "tabs": [{"key": "", "label": "Tất cả", "count": 9}, ...], "total": 9, "money": Money, "todo": 2 }
```

`GET /ctv/income`: `{ "money": Money, "bank": Bank, "user_bank": "chuỗi gốc", "pays": [Payout], "adjustments": [Adjustment] (chưa trả) }`
với `Payout = {id, created_at, ref, house_count, total}`.

`POST /houses`: ảnh không nhận được (trùng, hỏng, quá 20MB...) liệt kê trong `messages`. Không ảnh nào nhận: `house_id: null`.

## Hồ sơ (CTV chủ hồ sơ, admin, kế toán)

| Method | Path | Mô tả |
|---|---|---|
| GET | `/houses/{id}` | Chi tiết |
| GET | `/photos/{pid}` | Ảnh JPEG thu nhỏ 1568px; `?original=1`: ảnh gốc. Cần đăng nhập |
| GET | `/batches/{id}` | Phiếu chi (CTV: chỉ phiếu chưa huỷ của mình, không kèm lịch sử) |
| GET | `/batches/{id}/receipt` | Ảnh biên lai |

`GET /houses/{id}`:
```jsonc
{ "house": House, "photos": [Photo], "logs": [Log], "steps": [Step],
  "checklist": [{"key": "anh|address|contractor_name|contractor_phone|floors|stage", "label": "...", "ok": false, "message": "Thiếu địa chỉ công trình"}],
  "todo": [ "message", ... ],            // điều còn thiếu để gửi duyệt (checklist chưa đạt)
  "bad_fields": ["contractor_phone"],    // ô có giá trị nhưng sai dạng
  "issues": [{"index": 2, "problems": ["..."]}],   // index bắt đầu từ 1
  "ai_fields": ["address", "stage"],     // ô còn giữ nguyên giá trị AI đọc
  "valid": 5, "pay": 35000, "editable": true, "queue_left": 14 }
```

## Admin

| Method | Path | Mô tả |
|---|---|---|
| GET | `/admin/queue` | `{rows: [HouseRow]}` hồ sơ chờ duyệt |
| POST | `/admin/approve` | `{ids: [..]}` duyệt nhiều → `{message, approved}` |
| POST | `/houses/{id}/review` | `{action: "duyet\|tra_lai\|tu_choi", reason}` → `{message, next_id \| null, left}` |
| POST | `/houses/{id}/undo` | Đưa về chờ duyệt → `{message}` |
| PUT | `/houses/{id}/info` | Sửa thông tin công trình (chưa thanh toán) → `{message}` |
| GET | `/staff` | `{rows: [User]}` admin + kế toán |
| POST | `/staff` | `{login, name, role, password}` → `{message, user}` |
| POST | `/users/{id}/lock` | `{locked: bool}` → `{message}` |
| POST | `/users/{id}/password` | `{password}` → `{message}` |
| GET | `/settings` | `{price_min, price_more, min_photos, pay_min}` |
| PUT | `/settings` | cùng dạng → `{message, settings}` |

## Admin + kế toán

| Method | Path | Mô tả |
|---|---|---|
| PUT | `/houses/{id}/amount` | `{amount, reason}` (hồ sơ đã duyệt) → `{message}` |
| GET | `/houses?status=&q=&ctv=&from=&to=&page=` | Tổng quan |
| GET | `/ctvs?q=&page=` | Danh sách CTV |
| GET | `/ctvs/{id}` | 1 CTV |
| POST | `/ctvs/{id}/adjustments` | `{amount, reason}` → `{message}` |
| DELETE | `/adjustments/{id}` | Xoá khoản chưa trả → `{message}` |
| GET | `/batches` | `{rows: [Batch], total_paid}` (200 phiếu gần nhất) |
| GET | `/batches/{id}` | `{batch: Batch, rows: [HouseRow], adjustments: [Adjustment], logs: [Log], total}` (kể cả phiếu đã huỷ) |
| GET | `/reports?from=&to=&group=day\|week\|month` | Báo cáo |
| GET | `/reports/export?from=&to=` | File Excel chi tiết |

`GET /houses`: `{ rows, found, page, pages, counts: {status: n}, amounts: {status: tiền}, chips: {status: n theo bộ lọc},
ctv_total, ready_ctvs, ctv_name, filter: {status, q, ctv, from, to} }` (50 dòng/trang).

`GET /ctvs`: `{ rows: [{id, username, name, email, locked, created_at, houses, waiting, approved, paid, owed, last_at}], found, page, pages }`.

`GET /ctvs/{id}`: `{ user: User, bank: Bank, money: Money, rows: [HouseRow], counts: {status: n}, pays: [Payout], adjustments: [Adjustment] }`.

`GET /reports`: `{ from, to, group, periods: [{key, batches, ctvs, houses, total}], per_ctv: [{ctv_id, name, username, batches, houses, total}],
flow: {submitted, approved, returned, rejected}, new_ctvs, total, batches, houses }`.

## Kế toán

| Method | Path | Mô tả |
|---|---|---|
| GET | `/accounting` | `{ready: [PayRow], waiting: [PayRow], total}` |
| GET | `/accounting/export?format=xlsx\|csv&ids=1,2` | File chuyển khoản hàng loạt |
| POST | `/accounting/pay` | `{ref, items: [{ctv_id, total}]}` → `{message, paid: [batch_id], skipped}` |
| GET | `/payouts/{ctv_id}` | `{user, bank, rows: [HouseRow], adjustments, total, memo}` |
| POST | `/payouts/{ctv_id}` | `{house_ids, adjustment_ids, ref}` → `{message, batch_id}` |
| PUT | `/batches/{id}` | `{ref}` sửa mã giao dịch → `{message}` |
| POST | `/batches/{id}/cancel` | `{reason}` → `{message}` |
| POST | `/batches/{id}/receipt` | multipart `file` → `{message}` |

`PayRow`: `{ctv_id, name, username, bank, house_count, total}`.

Trả tiền gửi kèm đúng danh sách / tổng tiền kế toán đang thấy: khác dữ liệu hiện tại (vừa có hồ sơ mới duyệt, vừa thêm thưởng)
thì máy chủ từ chối (409) để tránh trả nhầm.
