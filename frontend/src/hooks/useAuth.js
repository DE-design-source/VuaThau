import { useCallback } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { clearAuthError, fetchMe, login, logout, renewToken, selectAuth } from '@/redux/slices/authSlice'

/** Trạng thái đăng nhập + thao tác. login/fetchMe trả promise (unwrap) để trang tự bắt lỗi. */
export default function useAuth() {
  const dispatch = useDispatch()
  const { user, token, status, error } = useSelector(selectAuth)

  return {
    user,
    token,
    status,
    error,
    role: user?.role,
    isAuthenticated: Boolean(token),
    isLoading: status === 'loading',
    login: useCallback((cred) => dispatch(login(cred)).unwrap(), [dispatch]),
    fetchMe: useCallback(() => dispatch(fetchMe()).unwrap(), [dispatch]),
    // API dùng JWT, không có phiên ở máy chủ: đăng xuất là bỏ token.
    logout: useCallback(() => dispatch(logout()), [dispatch]),
    renewToken: useCallback((t) => dispatch(renewToken(t)), [dispatch]),
    clearError: useCallback(() => dispatch(clearAuthError()), [dispatch]),
  }
}
