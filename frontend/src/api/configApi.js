import axiosClient from './axiosClient'

const configApi = {
  /** Hằng số chung với app: { statuses, status_order, min_photos, max_photos, pay_min, pay, stages, banks, reasons, google_client_id } */
  get: () => axiosClient.get('/config'),

  /** Admin: { price_min, price_more, min_photos, pay_min } */
  getSettings: () => axiosClient.get('/settings'),

  /** Admin: cùng dạng -> { message, settings } */
  saveSettings: (s) => axiosClient.put('/settings', s),
}

export default configApi
