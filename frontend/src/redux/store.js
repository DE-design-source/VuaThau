import { configureStore } from '@reduxjs/toolkit'
import authReducer, { logout, renewToken } from './slices/authSlice'
import { setSessionHandlers, setNtToken } from '@/api/ntClient'
import storageService from '@/services/storageService'
import { toast } from '@/services/toast'

export const store = configureStore({
  reducer: {
    auth: authReducer,
  },
  devTools: import.meta.env.DEV,
})

// Lưu token mỗi khi đổi (đăng nhập / đăng xuất / đổi mật khẩu).
let currentToken = store.getState().auth.token
store.subscribe(() => {
  const { token } = store.getState().auth
  if (token !== currentToken) {
    currentToken = token
    storageService.setToken(token)
    setNtToken(token)
  }
})

// Hết phiên, bị khoá: đăng xuất và báo lý do. Đổi mật khẩu: dùng token mới.
setSessionHandlers({
  end: (msg) => {
    if (store.getState().auth.token) {
      store.dispatch(logout())
      toast(msg)
    }
  },
  renew: (t) => store.dispatch(renewToken(t)),
})

export default store
