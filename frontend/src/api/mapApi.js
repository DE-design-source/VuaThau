import axiosClient, { cleanParams } from './axiosClient'
import storageService from '@/services/storageService'

const mapApi = {
  /** { points: [{id, lat, lng, status, ctv_id, ctv_name, address, contractor, source, at}], ctvs: [{id, name, phone, total, recent, dups, inbox, last}], dup_days, filter } */
  get: ({ ctv, status, from, to, province } = {}) => axiosClient.get('/map', { params: cleanParams({ ctv, status, from, to, province }) }),

  /** Tìm địa điểm theo địa chỉ (ô tìm trên bản đồ): { places: [{name, lat, lng}] } */
  searchPlace: (q) => axiosClient.get(storageService.getToken() ? '/geo/search' : '/public/geo/search', { params: { q } }),

  /** Admin: { enabled, running, at, added, rejected, error, total } lần đồng bộ Messenger gần nhất */
  inbox: () => axiosClient.get('/admin/inbox'),

  /** Admin: đồng bộ hồ sơ Messenger ngay */
  syncInbox: () => axiosClient.post('/admin/inbox/sync', {}, { timeout: 15 * 60 * 1000 }),
}

export default mapApi
