import axiosClient, { cleanParams } from './axiosClient'

const ctvApi = {
  /** Lọc: q, region (mã tỉnh, "-" = chưa rõ), kind (chức năng), bank ("co" | "chua") -> { rows, found, page, pages, facets } */
  list: ({ q, region, kind, bank, page } = {}) => axiosClient.get('/ctvs', { params: cleanParams({ q, region, kind, bank, page }) }),

  /** Admin: khu vực (mã tỉnh, '' = tự tính), chức năng, ghi chú -> { message } */
  setProfile: (id, { region, kind, note }) => axiosClient.put(`/ctvs/${id}/profile`, { region, kind, note }),

  /** Admin tạo tài khoản CTV (đăng nhập app bằng SĐT) -> { message, user } */
  add: ({ name, phone, password, bank_name, bank_no, bank_owner }) =>
    axiosClient.post('/ctvs', { name, phone, password, bank_name, bank_no, bank_owner }),

  /** { user, bank, money, rows, counts, pays, adjustments, profile: { region, region_auto, kind, note } } */
  get: (id) => axiosClient.get(`/ctvs/${id}`),

  /** Thưởng (> 0) / trừ (< 0) -> { message } */
  addAdjustment: (ctvId, { amount, reason }) => axiosClient.post(`/ctvs/${ctvId}/adjustments`, { amount, reason }),

  /** Xoá khoản chưa trả -> { message } */
  deleteAdjustment: (id) => axiosClient.delete(`/adjustments/${id}`),
}

export default ctvApi
