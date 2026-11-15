import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'
import authApi from '@/api/authApi'
import userApi from '@/api/userApi'
import storageService from '@/services/storageService'

const errorMessage = (err) => err?.message || 'Đã có lỗi xảy ra'

export const login = createAsyncThunk('auth/login', async (credentials, { rejectWithValue }) => {
  try {
    const res = await authApi.login(credentials)
    return res.data
  } catch (err) {
    return rejectWithValue(errorMessage(err))
  }
})

export const register = createAsyncThunk('auth/register', async (payload, { rejectWithValue }) => {
  try {
    const res = await authApi.register(payload)
    return res.data
  } catch (err) {
    return rejectWithValue(errorMessage(err))
  }
})

export const fetchMe = createAsyncThunk('auth/fetchMe', async (_, { rejectWithValue }) => {
  try {
    const res = await userApi.getMe()
    return res.data
  } catch (err) {
    return rejectWithValue(errorMessage(err))
  }
})

const initialState = {
  user: null,
  token: storageService.getToken(), // persisted by the store subscriber (see store.js)
  status: 'idle', // idle | loading | succeeded | failed
  error: null,
}

const setPending = (state) => {
  state.status = 'loading'
  state.error = null
}

const setRejected = (state, action) => {
  state.status = 'failed'
  state.error = action.payload ?? action.error.message
}

const setSession = (state, action) => {
  state.status = 'succeeded'
  state.user = action.payload.user
  state.token = action.payload.access_token
}

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    logout(state) {
      state.user = null
      state.token = null
      state.status = 'idle'
      state.error = null
    },
    clearAuthError(state) {
      state.error = null
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.pending, setPending)
      .addCase(login.fulfilled, setSession)
      .addCase(login.rejected, setRejected)
      .addCase(register.pending, setPending)
      .addCase(register.fulfilled, setSession)
      .addCase(register.rejected, setRejected)
      .addCase(fetchMe.pending, setPending)
      .addCase(fetchMe.fulfilled, (state, action) => {
        state.status = 'succeeded'
        state.user = action.payload
      })
      .addCase(fetchMe.rejected, setRejected)
  },
})

export const { logout, clearAuthError } = authSlice.actions

export const selectAuth = (state) => state.auth
export const selectUser = (state) => state.auth.user
export const selectToken = (state) => state.auth.token
export const selectIsAuthenticated = (state) => Boolean(state.auth.token)

export default authSlice.reducer
