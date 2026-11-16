import axiosClient, { getBlob, postForm } from './axiosClient'

const accountingApi = {
  /** { ready: [PayRow], waiting: [PayRow], total } */
  overview: () => axiosClient.get('/accounting'),

  /** File chuyển khoản hàng loạt (Blob). ids rỗng: mọi CTV đủ điều kiện. */
  exportFile: ({ format = 'xlsx', ids = [] } = {}) =>
    getBlob('/accounting/export', { format, ids: ids.length ? ids.join(',') : undefined }),

  /** Trả nhiều CTV: { ref, items: [{ctv_id, total}] } -> { message, paid: [batch_id], skipped } */
  payMany: ({ ref = '', items }) => axiosClient.post('/accounting/pay', { ref, items }),

  /** { user, bank, rows, adjustments, total, memo } */
  payout: (ctvId) => axiosClient.get(`/payouts/${ctvId}`),

  /** { house_ids, adjustment_ids, ref } -> { message, batch_id } */
  pay: (ctvId, { house_ids, adjustment_ids, ref = '' }) =>
    axiosClient.post(`/payouts/${ctvId}`, { house_ids, adjustment_ids, ref }),

  /** { rows: [Batch], total_paid } */
  batches: () => axiosClient.get('/batches'),

  /** { batch, rows, adjustments, logs, total } */
  batch: (id) => axiosClient.get(`/batches/${id}`),

  /** Sửa mã giao dịch -> { message } */
  updateRef: (id, ref) => axiosClient.put(`/batches/${id}`, { ref }),

  /** Huỷ phiếu chi -> { message } */
  cancel: (id, reason) => axiosClient.post(`/batches/${id}/cancel`, { reason }),

  /** Tải ảnh biên lai -> { message } */
  uploadReceipt: (id, file) => {
    const fd = new FormData()
    fd.append('file', file)
    return postForm(`/batches/${id}/receipt`, fd)
  },

  /** Ảnh biên lai (Blob) */
  receipt: (id) => getBlob(`/batches/${id}/receipt`),
}

export default accountingApi
