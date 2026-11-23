import axiosClient from './axiosClient'

// Nhà thầu (admin, kế toán): đăng ký qua form Lark + trích từ ảnh CTV chụp công trình.
const contractorApi = {
  /** { rows: [{ key, name, kind, role, phone, areas, exp, value_range, total, status, registered, photo, joined, updated }], form_url } */
  list: () => axiosClient.get('/contractors'),

  /** { ...row, profile: { contact, email, staff, fields, note... } | null, projects: [{ id, address, province_name, stage, at, cover_id, photos }] } */
  get: (key) => axiosClient.get(`/contractors/${encodeURIComponent(key)}`),

  /** Đọc lại form Lark ngay: { synced } */
  sync: () => axiosClient.post('/contractors/sync'),
}

export default contractorApi
