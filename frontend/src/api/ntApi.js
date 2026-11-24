import axiosClient from './axiosClient'

// Nhà thầu dùng app (admin)
const ntApi = {
  /** { rows: [{ id, phone, name, company, locked, projects, logs, last_log, created_at }] } */
  list: () => axiosClient.get('/admin/nt'),

  /** { phone, name, company } -> { message } */
  add: ({ phone, name, company }) => axiosClient.post('/admin/nt', { phone, name, company }),

  /** SĐT phụ (khớp thêm công trình CTV ghi số này): phones ['09...'] -> { alt_phones, message } */
  setAltPhones: (id, phones) => axiosClient.put(`/admin/nt/${id}/alt-phones`, { phones }),
}

export default ntApi
