import axios from 'axios'
import storageService from '@/services/storageService'

// Every backend response has the shape: { success: boolean, data?: any, error?: string }
const axiosClient = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
  headers: { 'Content-Type': 'application/json' },
  timeout: 15_000,
})

let onUnauthorized = null

/** Register a callback fired on any 401 response (e.g. dispatch logout). */
export function setUnauthorizedHandler(handler) {
  onUnauthorized = handler
}

axiosClient.interceptors.request.use((config) => {
  const token = storageService.getToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

axiosClient.interceptors.response.use(
  // Resolve with the envelope { success, data, error } instead of the axios response.
  (response) => response.data,
  (error) => {
    const status = error.response?.status
    if (status === 401) {
      storageService.clearToken()
      onUnauthorized?.()
    }
    const message = error.response?.data?.error || error.message || 'Request failed'
    return Promise.reject(Object.assign(new Error(message), { status }))
  },
)

export default axiosClient
