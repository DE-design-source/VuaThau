import axiosClient from './axiosClient'

// Nhà thầu dùng app (admin)
const ntApi = {
  /** { rows: [{ id, phone, name, company, locked, projects, logs, last_log, created_at }] } */
  list: () => axiosClient.get('/admin/nt'),

  /** { phone, name, company } -> { message } */
  add: ({ phone, name, company }) => axiosClient.post('/admin/nt', { phone, name, company }),
}

export default ntApi
