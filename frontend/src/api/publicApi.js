import axiosClient, { cleanParams } from './axiosClient'

// API không cần đăng nhập (người xem tự do: menu Bản đồ, Xem quy hoạch).
const publicApi = {
  /** { points: [{id, lat, lng, address, contractor, stage, province, at, last_at, cover_url}], provinces, stale_days } (không tên CTV) */
  map: ({ province } = {}) => axiosClient.get('/public/map', { params: cleanParams({ province }) }),

  /** Chi tiết 1 công trình: { id, address, province_name, stage, floors, contractor, at, lat, lng, photos: [{url, time}] } */
  project: (id) => axiosClient.get(`/public/projects/${id}`),

  /** Báo cáo nhật ký thi công gửi chủ nhà (/bc/:code): { company, verified, project, address, note, taken_at, gps, photos: [url], videos, status, progress } */
  report: (code) => axiosClient.get(`/public/bc/${encodeURIComponent(code)}`),

  /** Tìm địa điểm (chỉ địa điểm): { sites: [], places: [{name, lat, lng}] } */
  searchPlace: (q) => axiosClient.get('/public/geo/search', { params: { q } }),
}

export default publicApi
