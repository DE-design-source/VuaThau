import axiosClient, { cleanParams, getBlob } from './axiosClient'

const houseApi = {
  /** Tổng quan: { rows, found, page, pages, counts, amounts, chips, ctv_total, ready_ctvs, ctv_name, filter } */
  list: ({ status, q, ctv, province, from, to, page } = {}) =>
    axiosClient.get('/houses', { params: cleanParams({ status, q, ctv, province, from, to, page }) }),

  /** { house, photos, logs, steps, checklist, todo, bad_fields, issues, ai_fields, valid, pay, editable, queue_left } */
  get: (id) => axiosClient.get(`/houses/${id}`),

  /** Admin: action duyet | tra_lai | tu_choi -> { message, next_id, left } */
  review: (id, { action, reason = '' }) => axiosClient.post(`/houses/${id}/review`, { action, reason }),

  /** Admin: đưa về chờ duyệt -> { message } */
  undo: (id) => axiosClient.post(`/houses/${id}/undo`),

  /** Admin: sửa thông tin công trình -> { message } */
  updateInfo: (id, { address, floors, stage, contractor_name, contractor_phone }) =>
    axiosClient.put(`/houses/${id}/info`, { address, floors, stage, contractor_name, contractor_phone }),

  /** Admin + kế toán: sửa tiền hồ sơ đã duyệt -> { message } */
  setAmount: (id, { amount, reason }) => axiosClient.put(`/houses/${id}/amount`, { amount, reason }),

  /** Admin: loại chụp ngoai | trong_ngoai (tiền tính lại) -> { message } */
  setScope: (id, scope) => axiosClient.put(`/houses/${id}/scope`, { scope }),

  /** Ảnh JPEG (Blob); original: ảnh gốc. */
  photo: (pid, { original = false } = {}) => getBlob(`/photos/${pid}`, original ? { original: 1 } : undefined),
}

export default houseApi
