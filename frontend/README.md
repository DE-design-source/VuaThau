# Vua Thầu: web quản lý nghiệm thu (admin + kế toán)

React + Vite, Redux Toolkit, React Router, axios.

**Ai dùng gì**

| Người dùng | Dùng | Ở đâu |
|---|---|---|
| Cộng tác viên | App điện thoại (Flutter) | repo app |
| Admin, kế toán | Web này | thư mục `frontend/` |

Web gọi **backend Go** của repo (`../backend`) qua REST API `/api/v1`, đúng API mà app CTV dùng, cùng một CSDL nên dữ liệu
thông nhau. CTV nộp hồ sơ trên app thì admin thấy ngay ở mục Duyệt hồ sơ. Admin duyệt hoặc trả lại thì CTV thấy trên app.
Kế toán trả tiền, tải biên lai thì CTV thấy trong Thu nhập.

Hợp đồng API: `../backend/docs/nghiemthu-api.md`. Mọi response là envelope `{success, data, error}`; đăng nhập trả JWT,
gửi `Authorization: Bearer <token>`. Khi dev, Vite chuyển `/api` (và `/ws`) sang `http://localhost:8080`, nên không cần CORS.

## Chức năng

**Admin**
- Duyệt hồ sơ: hàng chờ, duyệt nhiều hồ sơ một lúc, duyệt lần lượt (xong tự sang hồ sơ kế tiếp)
- Chi tiết hồ sơ: ảnh kèm lỗi AI (xem lớn, tải ảnh gốc, giờ chụp, vị trí), cảnh báo trùng nhà, trả lại hoặc từ chối kèm lý do mẫu,
  hoàn tác, sửa thông tin công trình (ghi lịch sử), sửa tiền
- Nhân viên: thêm admin/kế toán, khoá, đặt lại mật khẩu
- Cấu hình: giá hồ sơ, số ảnh tối thiểu, số hồ sơ để được thanh toán (app CTV tự cập nhật)

**Kế toán**
- Thanh toán: CTV đủ điều kiện, tải Excel/CSV chuyển khoản hàng loạt, xác nhận đã trả nhiều CTV một lúc
- Thanh toán từng CTV: số TK, số tiền, nội dung chuyển khoản (bấm để chép), mã giao dịch, thưởng/trừ
- Phiếu chi: sửa mã giao dịch, tải ảnh biên lai, huỷ phiếu ghi nhầm

**Cả hai**: Hồ sơ (tìm, lọc trạng thái, lọc ngày, phân trang), Cộng tác viên (tiền, hồ sơ, các lần nhận tiền, thưởng/trừ),
Phiếu chi, Báo cáo (theo ngày/tuần/tháng, theo CTV, luồng duyệt, xuất Excel), đổi mật khẩu.

Tài khoản CTV đăng nhập web sẽ bị từ chối (CTV dùng app).

## Chạy khi dev

1. Chạy backend Go ở cổng 8080 (xem `../backend`).
2. Chạy web:

```bash
npm install
# tuỳ chọn: ../.env chung ở gốc repo (Vite đọc ở đó; không có trong git)
npm run dev            # http://localhost:5173, /api chuyển tới http://localhost:8080
npm run lint
npm run build          # ra dist/
```

## Triển khai

Phục vụ `dist/` và chuyển `/api/*` (cùng tên miền) sang backend Go. Muốn gọi API ở tên miền khác thì build với
`VITE_API_URL=https://.../api/v1` (backend phải cho phép CORS).

## Biến môi trường

| Biến | Mặc định | Mô tả |
|---|---|---|
| `VITE_API_URL` | `/api/v1` | Địa chỉ API backend Go (lúc build) |
| `VITE_WS_URL` | cùng tên miền | Địa chỉ WebSocket (scaffold, chưa dùng cho nghiệm thu) |

## Cấu trúc

```
src/
  api/axiosClient.js   axios: baseURL /api/v1, Bearer token, trả envelope; 401 -> đăng xuất; getBlob (ảnh, Excel), postForm (multipart)
  api/*Api.js          mỗi miền 1 module: authApi, userApi (/me, khoá, mật khẩu), configApi (/config, /settings),
                       houseApi (/houses, /photos), adminApi (/admin/queue, /staff), ctvApi (/ctvs, /adjustments),
                       accountingApi (/accounting, /payouts, /batches), reportApi (/reports)
  hooks/usePage.js     usePage(fetcher, deps) -> {data, err, busy, reload}
  hooks/useAct.js      gửi thao tác ghi, báo data.message, rồi tải lại hoặc sang trang khác (vd duyệt xong sang next_id)
  hooks/useCfg.js      hằng số chung từ GET /config (trạng thái, giai đoạn, lý do mẫu, pay_min)
  pages/               /queue, /houses, /houses/:id, /ctvs, /ctvs/:id, /staff, /settings, /accounting,
                       /payouts/:ctvId, /batches, /batches/:id, /reports, /me
  components/ui/nt.jsx bảng hồ sơ, lưới ảnh + xem lớn, ảnh cần đăng nhập (AuthImage), nhãn trạng thái, thẻ số liệu, phân trang, thông báo
  redux/               đăng nhập (chỉ admin, kế toán), lưu token qua storageService
```

Trang chủ `/` chuyển admin tới `/queue`, kế toán tới `/accounting`.