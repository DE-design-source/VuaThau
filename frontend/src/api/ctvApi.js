import axiosClient, { cleanParams } from './axiosClient'

const ctvApi = {
  /** { rows, found, page, pages } */
  list: ({ q, page } = {}) => axiosClient.get('/ctvs', { params: cleanParams({ q, page }) }),

  /** { user, bank, money, rows, counts, pays, adjustments } */
  get: (id) => axiosClient.get(`/ctvs/${id}`),

  /** Thưởng (> 0) / trừ (< 0) -> { message } */
  addAdjustment: (ctvId, { amount, reason }) => axiosClient.post(`/ctvs/${ctvId}/adjustments`, { amount, reason }),

  /** Xoá khoản chưa trả -> { message } */
  deleteAdjustment: (id) => axiosClient.delete(`/adjustments/${id}`),
}

export default ctvApi
