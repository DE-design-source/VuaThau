import axiosClient from './axiosClient'

const adminApi = {
  /** { rows: [HouseRow] } hồ sơ chờ duyệt */
  queue: () => axiosClient.get('/admin/queue'),

  /** Duyệt nhiều -> { message, approved } */
  approve: (ids) => axiosClient.post('/admin/approve', { ids }),

  /** { rows: [User] } admin + kế toán */
  staff: () => axiosClient.get('/staff'),

  /** { login, name, role, password } -> { message, user } */
  addStaff: ({ login, name, role, password }) => axiosClient.post('/staff', { login, name, role, password }),
}

export default adminApi
