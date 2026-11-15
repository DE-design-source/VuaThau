import { useCallback } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import {
  clearAuthError,
  fetchMe,
  login,
  logout,
  register,
  selectAuth,
} from '@/redux/slices/authSlice'

/**
 * Auth state + actions. login/register/fetchMe return a promise that
 * resolves with the payload or rejects with the error message (via unwrap()).
 */
export default function useAuth() {
  const dispatch = useDispatch()
  const { user, token, status, error } = useSelector(selectAuth)

  return {
    user,
    token,
    status,
    error,
    isAuthenticated: Boolean(token),
    isLoading: status === 'loading',
    login: useCallback((credentials) => dispatch(login(credentials)).unwrap(), [dispatch]),
    register: useCallback((payload) => dispatch(register(payload)).unwrap(), [dispatch]),
    fetchMe: useCallback(() => dispatch(fetchMe()).unwrap(), [dispatch]),
    logout: useCallback(() => dispatch(logout()), [dispatch]),
    clearError: useCallback(() => dispatch(clearAuthError()), [dispatch]),
  }
}
