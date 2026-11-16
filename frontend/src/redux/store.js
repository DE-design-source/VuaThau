import { configureStore } from '@reduxjs/toolkit'
import authReducer, { logout } from './slices/authSlice'
import { setUnauthorizedHandler } from '@/api/axiosClient'
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
  }
})

// 401 (hết phiên, bị khoá, đổi mật khẩu ở nơi khác): đăng xuất và báo câu của máy chủ.
setUnauthorizedHandler((msg) => {
  if (store.getState().auth.token) {
    store.dispatch(logout())
    toast(msg)
  }
})

export default store
