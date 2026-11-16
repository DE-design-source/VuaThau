import axiosClient, { cleanParams } from './axiosClient'

// Trang sale: chỉ thông tin công trình + nhà thầu + ảnh. Không có CTV, không có tiền.
const saleApi = {
  /** { points: [{id, lat, lng, address, contractor, stage, province, at}], provinces: [{code, name, merged, total}], province } */
  map: ({ province, ctv } = {}) => axiosClient.get('/sale/map', { params: cleanParams({ province, ctv }) }),

  /** { rows: [{id, address, lat, lng, province, province_name, floors, stage, contractor_name, contractor_phone, at, cover_id, photos}], found, page, pages } */
  projects: ({ q, province, contractor, page } = {}) =>
    axiosClient.get('/sale/projects', { params: cleanParams({ q, province, contractor, page }) }),

  /** 1 dự án + photo_list: [{id, time, lat, lng}] */
  project: (id) => axiosClient.get(`/sale/projects/${id}`),

  /** { rows: [{key, name, phone, total, provinces, last}] } */
  contractors: ({ q } = {}) => axiosClient.get('/sale/contractors', { params: cleanParams({ q }) }),
}

export default saleApi
