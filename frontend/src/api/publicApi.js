import axiosClient, { cleanParams } from './axiosClient'

// API không cần đăng nhập (người xem tự do: menu Bản đồ, Xem quy hoạch).
const publicApi = {
  /** { points: [{id, lat, lng, address, contractor, stage, province, at, last_at, cover_url}], provinces, stale_days } (không tên CTV) */
  map: ({ province } = {}) => axiosClient.get('/public/map', { params: cleanParams({ province }) }),

  /** Tìm địa điểm (chỉ địa điểm): { sites: [], places: [{name, lat, lng}] } */
  searchPlace: (q) => axiosClient.get('/public/geo/search', { params: { q } }),
}

export default publicApi
