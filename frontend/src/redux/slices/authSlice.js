// Đăng nhập web quản lý: chỉ admin và kế toán. CTV dùng app điện thoại.
import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'
import ntClient, { setNtToken } from '@/api/ntClient'
import storageService from '@/services/storageService'

export const STAFF_HOME = { admin: '/duyet', ketoan: '/ke-toan' }

export const login = createAsyncThunk('auth/login', async ({ phone, pw }, { rejectWithValue }) => {
  try {
    const j = await ntClient.post('/login', { phone, pw })
    if (!j.token) return rejectWithValue(j.msgs?.[0] || 'Chưa đăng nhập được, thử lại nhé.')
    if (!Object.values(STAFF_HOME).includes(j.url)) {
      return rejectWithValue('Tài khoản cộng tác viên dùng app điện thoại. Web này dành cho admin và kế toán.')
    }
    setNtToken(j.token)
    const me = await ntClient.get('/toi')
    return { token: j.token, user: me.user, msgs: j.msgs }
  } catch (err) {
    setNtToken(null)
    return rejectWithValue(err.message)
  }
})

export const fetchMe = createAsyncThunk('auth/fetchMe', async (_, { rejectWithValue }) => {
  try {
    const me = await ntClient.get('/toi')
    return me.user
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
      })
  },
})

export const { logout, renewToken, clearAuthError } = authSlice.actions
export const selectAuth = (state) => state.auth
export const selectUser = (state) => state.auth.user
export const selectRole = (state) => state.auth.user?.role
export default authSlice.reducer
