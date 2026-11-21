// Lớp bản đồ (layout 21/11/2026): hằng số + hook nhớ lớp đang bật. Giao diện ở components/explore/layers.jsx.
import { useState } from 'react'

export const LAYER_META = {
  pending: { label: 'Chưa tới lúc chụp', color: '#F5B301' },
  shoot: { label: 'Cần chụp', color: '#22C55E' },
  dezon: { label: 'Dezon', color: '#1A56DB' },
  ctvUpdate: { label: 'CTV cập nhật', color: '#EC4899' },
  ctvSales: { label: 'CTV kinh doanh', color: '#DC2626' },
  customer: { label: 'Khách hàng', color: '#EA580C' },
}
export const ALL_LAYERS = { pending: true, shoot: true, dezon: true, ctvUpdate: true, ctvSales: true, ctvActive: true, ctvReg: true, customer: true }
/** Loại khách hàng (máy chủ: nha_pho | biet_thu | can_ho) */
export const CUSTOMER_KINDS = { nha_pho: 'Nhà phố', biet_thu: 'Biệt thự', can_ho: 'Căn hộ' }
const KEY = 'vuathau.mapLayers'

/** Lớp đang bật, nhớ theo trình duyệt (mở lại trang vẫn giữ). */
export function useLayers() {
  const [layers, setLayers] = useState(() => {
    try {
      return { ...ALL_LAYERS, ...JSON.parse(localStorage.getItem(KEY) || '{}') }
    } catch {
      return ALL_LAYERS
    }
  })
  const set = (patch) =>
    setLayers((l) => {
      const next = { ...l, ...patch }
      try {
        localStorage.setItem(KEY, JSON.stringify(next))
      } catch {
        /* không lưu được: chỉ mất ghi nhớ */
      }
      return next
    })
  return [layers, set]
}

export const ctvKey = (c) => (c.type === 'sales' ? 'ctvSales' : 'ctvUpdate')
export const ctvVisible = (c, L) => L[ctvKey(c)] && L[c.status === 'active' ? 'ctvActive' : 'ctvReg']
export const ctvTypeLabel = (c) => (c.type === 'sales' ? 'Kinh doanh' : 'Cập nhật')
export const ctvStatusLabel = (c) => (c.status === 'active' ? 'Tích cực' : 'Đã đăng ký')

