import axiosClient from './axiosClient'

const configApi = {
  /** Hằng số chung với app: { statuses, status_order, min_photos, max_photos, pay_min, pay, stages, banks, reasons, google_client_id } */
  get: () => axiosClient.get('/config'),

  /** Admin: { price_min, price_more, min_photos, pay_min } */
  getSettings: () => axiosClient.get('/settings'),

  /** Admin: cùng dạng -> { message, settings } */
  saveSettings: (s) => axiosClient.put('/settings', s),

  /** Admin: quy tắc duyệt ảnh cho AI -> { rules, at, default_prompt, max } */
  getAiRules: () => axiosClient.get('/settings/ai-rules'),

  /** Admin: lưu quy tắc ('' = bỏ, dùng lời dặn mặc định) -> { message, rules } */
  saveAiRules: (rules) => axiosClient.put('/settings/ai-rules', { rules }),

  /** Quy tắc xếp thứ tự ảnh công trình: sign_first | time | sent */
  savePhotoOrder: (order) => axiosClient.put('/settings/photo-order', { order }),

  /** Admin: danh sách chức năng CTV -> { message, kinds } */
  saveKinds: (kinds) => axiosClient.put('/ctv-kinds', { kinds }),
}

export default configApi
