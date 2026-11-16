// Đăng nhập web quản lý: chỉ admin và kế toán. CTV dùng app điện thoại.
import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'
import authApi from '@/api/authApi'
import userApi from '@/api/userApi'
import storageService from '@/services/storageService'

export const STAFF_HOME = { admin: '/queue', ketoan: '/accounting' }

export const CTV_REFUSED = 'Tài khoản cộng tác viên dùng app điện thoại. Web này dành cho admin và kế toán.'

export const login = createAsyncThunk('auth/login', async ({ login, password }, { rejectWithValue }) => {
  try {
    const { data } = await authApi.login({ login, password })
    if (!data?.access_token) return rejectWithValue('Chưa đăng nhập được, thử lại nhé.')
    if (!STAFF_HOME[data.user?.role]) return rejectWithValue(CTV_REFUSED)
    return { token: data.access_token, user: data.user }
  } catch (err) {
    return rejectWithValue(err.message)
  }
})

export const fetchMe = createAsyncThunk('auth/fetchMe', async (_, { rejectWithValue }) => {
  try {
    const { data } = await userApi.getMe()
    if (!STAFF_HOME[data?.user?.role]) return rejectWithValue(CTV_REFUSED)
    return data.user
  } catch (err) {
    return rejectWithValue(err.message)
  }
})

const authSlice = createSlice({
  name: 'auth',
  initialState: {
    user: null,
    token: storageService.getToken(), // lưu lại qua store.subscribe (store.js)
    status: 'idle', // idle | loading | succeeded | failed
    error: null,
  },
  reducers: {
    logout(state) {
      state.user = null
      state.token = null
      state.status = 'idle'
      state.error = null
    },
    /** Đổi mật khẩu: máy chủ trả access_token mới, token cũ hết hiệu lực. */
    renewToken(state, action) {
      state.token = action.payload
    },
    clearAuthError(state) {
      state.error = null
    },
  },
  extraReducers: (b) => {
    b.addCase(login.pending, (s) => {
      s.status = 'loading'
      s.error = null
    })
      .addCase(login.fulfilled, (s, a) => {
        s.status = 'succeeded'
        s.user = a.payload.user
        s.token = a.payload.token
      })
      .addCase(login.rejected, (s, a) => {
        s.status = 'failed'
        s.error = a.payload ?? a.error.message
      })
      .addCase(fetchMe.fulfilled, (s, a) => {
        s.status = 'succeeded'
        s.user = a.payload
      })
      .addCase(fetchMe.rejected, (s, a) => {
        s.status = 'failed'
        s.error = a.payload ?? a.error.message
        // Token của CTV (vd lưu từ trước): không cho dùng web
        if (a.payload === CTV_REFUSED) {
          s.token = null
          s.user = null
        }
      })
  },
})

export const { logout, renewToken, clearAuthError } = authSlice.actions
export const selectAuth = (state) => state.auth
export const selectUser = (state) => state.auth.user
export const selectToken = (state) => state.auth.token
export const selectRole = (state) => state.auth.user?.role
export default authSlice.reducer
