import axiosClient from './axiosClient'

// Resolves with { success, data: { user, access_token, expires_in } }
const authApi = {
  /** login: SĐT (CTV) hoặc tên đăng nhập (nhân viên). */
  login: ({ login, password }) => axiosClient.post('/auth/login', { login, password }),
}

export default authApi
