import axiosClient, { cleanParams } from './axiosClient'

const dezonApi = {
  /** Công trình nhà thầu đăng trên dezon.vn đã có toạ độ:
   * { rows: [{id, url, title, contractor, contractor_url, cover, posted_at, lat, lng, province, geo_src, geo_level}], source } */
  projects: ({ province } = {}) => axiosClient.get('/dezon/projects', { params: cleanParams({ province }) }),
}

export default dezonApi
