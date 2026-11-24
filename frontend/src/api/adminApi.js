import axiosClient from './axiosClient'

const adminApi = {
  /** { rows: [HouseRow] } hồ sơ chờ duyệt */
  queue: () => axiosClient.get('/admin/queue'),

  /** Duyệt nhiều -> { message, approved } */
  approve: (ids) => axiosClient.post('/admin/approve', { ids }),

  /** Đánh giá app: { items: [{ id, name, phone, role, stars, comment, photos, version, platform, created_at }], count, avg, stars[0..5] } */
  feedback: () => axiosClient.get('/admin/feedback'),

  /** { rows: [User] } admin + kế toán */
  staff: () => axiosClient.get('/staff'),

  /** { login, name, role, password } -> { message, user } */
  addStaff: ({ login, name, role, password }) => axiosClient.post('/staff', { login, name, role, password }),
}

export default adminApi
