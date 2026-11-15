import { configureStore } from '@reduxjs/toolkit'
import authReducer, { logout } from './slices/authSlice'
import { setUnauthorizedHandler } from '@/api/axiosClient'
import storageService from '@/services/storageService'

export const store = configureStore({
  reducer: {
    auth: authReducer,
  },
  devTools: import.meta.env.DEV,
})

// Persist the auth token whenever it changes (login / register / logout).
let currentToken = store.getState().auth.token
store.subscribe(() => {
  const { token } = store.getState().auth
  if (token !== currentToken) {
    currentToken = token
    storageService.setToken(token)
  }
})

// Any 401 from the API logs the user out.
setUnauthorizedHandler(() => {
  if (store.getState().auth.token) store.dispatch(logout())
})

export default store
