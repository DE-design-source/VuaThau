import axiosClient, { cleanParams, getBlob } from './axiosClient'

const reportApi = {
  /** group: day | week | month -> { from, to, group, periods, per_ctv, flow, new_ctvs, total, batches, houses } */
  get: ({ from, to, group } = {}) => axiosClient.get('/reports', { params: cleanParams({ from, to, group }) }),

  /** File Excel chi tiết (Blob) */
  exportFile: ({ from, to } = {}) => getBlob('/reports/export', { from, to }),
}

export default reportApi
