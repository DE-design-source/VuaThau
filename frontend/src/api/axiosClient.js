import axios from 'axios'
import storageService from '@/services/storageService'

// Every backend response has the shape: { success: boolean, data?: any, error?: string }
// Binary endpoints (ảnh, biên lai, Excel) dùng responseType 'blob' và resolve thẳng ra Blob.
const axiosClient = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
  headers: { 'Content-Type': 'application/json' },
  timeout: 15_000,
})

let onUnauthorized = null

/** Register a callback fired on any 401 response (e.g. dispatch logout). Receives the server message. */
export function setUnauthorizedHandler(handler) {
  onUnauthorized = handler
}

/** Bỏ tham số rỗng ('' / null / undefined) khỏi query. */
export function cleanParams(params = {}) {
  return Object.fromEntries(Object.entries(params).filter(([, v]) => v !== '' && v !== null && v !== undefined))
}

/** Body JSON của response lỗi; với request blob thì đọc Blob ra text rồi parse. */
async function errorBody(data) {
  if (typeof Blob !== 'undefined' && data instanceof Blob) {
    try {
      return JSON.parse(await data.text())
    } catch {
      return null
    }
  }
  return data && typeof data === 'object' ? data : null
}

axiosClient.interceptors.request.use((config) => {
  const token = storageService.getToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

axiosClient.interceptors.response.use(
  // Resolve with the envelope { success, data, error } (or the Blob for binary requests).
  async (response) => {
    const { data } = response
    // Request blob mà máy chủ trả JSON (envelope lỗi): báo lỗi thay vì đưa Blob JSON ra như file.
    if (data instanceof Blob && (data.type || '').includes('application/json')) {
      const body = await errorBody(data)
      if (body && body.success === false) return Promise.reject(new Error(body.error || 'Có lỗi xảy ra.'))
    }
    return data
  },
  async (error) => {
    const status = error.response?.status
    const body = await errorBody(error.response?.data)
    let message = body?.error
    if (!message) {
      if (!error.response) {
        message =
          error.code === 'ECONNABORTED'
            ? 'Máy chủ phản hồi quá lâu. Thử lại sau ít phút.'
            : 'Mất kết nối tới máy chủ. Kiểm tra mạng rồi thử lại.'
      } else if (status === 401) message = 'Hết phiên đăng nhập, đăng nhập lại nhé.'
      else message = `Có lỗi xảy ra (mã ${status}).`
    }
    if (status === 401) {
      storageService.clearToken()
      onUnauthorized?.(message)
    }
    return Promise.reject(Object.assign(new Error(message), { status }))
  },
)

/** GET file cần đăng nhập (ảnh, biên lai, Excel) -> Blob. */
export const getBlob = (url, params) =>
  axiosClient.get(url, { params: params && cleanParams(params), responseType: 'blob', timeout: 60_000 })

/** POST multipart (FormData). Ghi đè Content-Type JSON mặc định để axios gửi đúng boundary. */
export const postForm = (url, formData) =>
  axiosClient.post(url, formData, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 60_000 })

export default axiosClient
