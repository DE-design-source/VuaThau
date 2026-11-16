# Vua Thầu: web quản lý nghiệm thu (admin + kế toán)

React + Vite, Redux Toolkit, React Router.

**Ai dùng gì**

| Người dùng | Dùng | Ở đâu |
|---|---|---|
| Cộng tác viên | App Android | repo `app-Vua-Thau-` |
| Admin, kế toán | Web này | repo này, thư mục `frontend/` |

App và web đều gọi **cùng một máy chủ Nghiệm thu** (`nghiemthu.vuathau.com.vn`, mã nguồn ở repo `vuathau-nghiemthu`),
nên chung dữ liệu theo thời gian thực. CTV nộp hồ sơ trên app thì admin thấy ngay ở mục Duyệt hồ sơ. Admin duyệt hoặc trả lại
thì CTV thấy trên app. Kế toán trả tiền, tải biên lai thì CTV thấy trong Thu nhập.

Web gửi header `X-App: 1` nên máy chủ trả JSON, đăng nhập bằng token `Bearer` như app. Mọi request đi qua đường dẫn `/nt`
trên cùng tên miền (Vite proxy khi dev, Caddy khi chạy thật), nên không phải sửa máy chủ, cũng không cần CORS.

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

## Lệnh

```bash
npm install
cp .env.example .env   # tuỳ chọn
npm run dev            # http://localhost:5173, /nt chuyển tới máy chủ thật (NT_PROXY_TARGET)
npm run lint
npm run build          # ra dist/
```

Chạy với máy chủ Nghiệm thu ở máy mình (bản demo có sẵn tài khoản mẫu):
`DEMO=1 uvicorn app:app --port 8000` trong repo `vuathau-nghiemthu`, rồi đặt `NT_PROXY_TARGET=http://localhost:8000`.

## Triển khai

Xem `../deploy/Caddyfile.quanly`: Caddy phục vụ `dist/` và chuyển `/nt/*` sang container `nghiemthu-web:8000`.

## Biến môi trường

| Biến | Mặc định | Mô tả |
|---|---|---|
| `VITE_NT_URL` | `/nt` | Đường dẫn tới máy chủ Nghiệm thu (lúc build) |
| `NT_PROXY_TARGET` | `https://nghiemthu.vuathau.com.vn` | Dev: máy chủ mà Vite chuyển `/nt` tới |
| `VITE_API_URL` | `/api/v1` | Backend Go của repo (chưa dùng cho nghiệm thu) |

## Cấu trúc

```
src/
  api/ntClient.js      gọi máy chủ Nghiệm thu (X-App, token, tải file, ảnh cần đăng nhập)
  hooks/usePage.js     dữ liệu 1 trang; useAct.js gửi thao tác rồi đi tới url máy chủ trả về; useCfg.js hằng số chung
  pages/               mỗi trang ứng với 1 route của máy chủ (/duyet, /nha/:id, /ke-toan, /dot/:id...)
  components/ui/nt.jsx bảng hồ sơ, lưới ảnh + xem lớn, nhãn trạng thái, thẻ số liệu, phân trang, thông báo
  redux/               đăng nhập (chỉ admin, kế toán)
```

Route của web trùng đường dẫn máy chủ, nên `url` máy chủ trả sau mỗi thao tác (vd duyệt xong sang `/nha/12`) dùng thẳng được.
