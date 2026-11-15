# Vuathau Frontend

React + Vite (JavaScript/JSX), Redux Toolkit, React Router, Axios.
Kết nối tới backend Go (`../backend`) qua REST `/api/v1` và WebSocket `/ws`.

## Yêu cầu

- Node.js >= 20 (đã thử với 22.14), npm >= 10
- Backend chạy tại `http://localhost:8080` (Vite dev server proxy `/api` và `/ws` sang đó)

## Lệnh

```bash
npm install        # cài dependencies
cp .env.example .env   # (tuỳ chọn) cấu hình biến môi trường
npm run dev        # dev server: http://localhost:5173
npm run lint       # ESLint
npm run build      # build production ra dist/
npm run preview    # xem thử bản build
```

## Biến môi trường

| Biến           | Mặc định  | Mô tả                                                        |
| -------------- | --------- | ------------------------------------------------------------ |
| `VITE_API_URL` | `/api/v1` | Base URL cho REST API                                        |
| `VITE_WS_URL`  | (trống)   | URL WebSocket, bỏ trống để suy ra từ `window.location`       |

## Cấu trúc

```
src/
  api/          axiosClient (interceptor token/401) + authApi, userApi, messageApi
  assets/       ảnh, logo
  components/
    layout/     MainLayout, Header, Footer
    ui/         Button, Input, Loader (export qua index.js)
  context/      ThemeContext + ThemeProvider (light/dark)
  data/         dữ liệu tĩnh (navLinks)
  hooks/        useAuth, useTheme, useDebounce, useWebSocket
  pages/        HomePage, LoginPage, NotFoundPage
  redux/        store + slices/authSlice
  services/     storageService (localStorage an toàn), socketService (WebSocket + auto-reconnect)
  utils/        formatDate, validators
  App.jsx       khai báo routes
  main.jsx      entry: Provider > ThemeProvider > BrowserRouter > App
  index.css     style chung + biến CSS cho light/dark
```

Import tuyệt đối dùng alias `@` -> `src` (ví dụ `import useAuth from '@/hooks/useAuth'`).

## Ghi chú

- Mọi response từ backend có dạng `{ success, data, error }`; `axiosClient` trả về đúng object này (đã bỏ lớp axios response).
- Token JWT lưu trong localStorage, tự gắn header `Authorization: Bearer <token>`; gặp 401 sẽ tự đăng xuất.
- WebSocket kết nối `/ws?token=<jwt>` khi đã đăng nhập; server đẩy sự kiện `{ type: "message:new", data }`.
