import axiosClient from './axiosClient'

// Resolves with { success, data: { user, access_token, expires_in } }
const authApi = {
  /** login: SĐT (CTV) hoặc tên đăng nhập (nhân viên). */
  login: ({ login, password }) => axiosClient.post('/auth/login', { login, password }),
  /** CTV / nhà thầu vào bằng SĐT: { registered, phone } (as = 'ctv' | 'nhathau'; chọn nhầm vai trò -> lỗi 409 kèm hướng dẫn). */
  phone: ({ phone, as }) => axiosClient.post('/auth/phone', { phone, as }),
}

export default authApi
