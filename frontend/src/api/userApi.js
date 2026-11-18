import axiosClient from './axiosClient'

const userApi = {
  /** { user, bank, todo } */
  getMe: () => axiosClient.get('/me'),

  /** { message, access_token, expires_in }: token cũ hết hiệu lực, phải lưu token mới. */
  changePassword: ({ old, new: next }) => axiosClient.put('/me/password', { old, new: next }),

  /** Admin: khoá / mở khoá tài khoản (CTV hoặc nhân viên) -> { message } */
  lock: (id, locked) => axiosClient.post(`/users/${id}/lock`, { locked: Boolean(locked) }),

  /** Admin: đặt lại mật khẩu -> { message } */
  setPassword: (id, password) => axiosClient.post(`/users/${id}/password`, { password }),
}

export default userApi
