import axiosClient from './axiosClient'

const userApi = {
  getMe: () => axiosClient.get('/users/me'),

  list: ({ limit = 20, offset = 0 } = {}) => axiosClient.get('/users', { params: { limit, offset } }),

  getById: (id) => axiosClient.get(`/users/${id}`),
}

export default userApi
