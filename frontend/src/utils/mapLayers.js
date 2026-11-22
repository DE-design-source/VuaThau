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
/** Trạng thái giao tiếp (sao bên Dezon Pro) */
export const RATING_LABEL = ['Chưa đánh giá', 'Rất thấp', 'Thấp', 'Trung bình', 'Cao', 'Rất cao']

/** Bộ lọc tab Khách hàng (góp ý 22/11/2026): người phụ trách, tỉnh thành, nguồn, trạng thái giao tiếp, ngày tạo. */
export const NO_CUS_FILTER = { owner: '', province: '', source: '', rating: '', date: '', from: '', to: '' }

/** Ngày yyyy-mm-dd theo giờ Việt Nam */
const vnDay = (d) => new Date(d).toLocaleDateString('en-CA', { timeZone: 'Asia/Ho_Chi_Minh' })

/** Khách có khớp bộ lọc f không (không xét ô tìm). */
export function cusMatch(k, f) {
  if (f.owner && k.owner !== f.owner) return false
  if (f.province && (k.province || '') !== (f.province === '-' ? '' : f.province)) return false
  if (f.source && (k.source || '') !== (f.source === '-' ? '' : f.source)) return false
  if (f.rating !== '' && f.rating != null && String(k.rating || 0) !== String(f.rating)) return false
  if (f.date) {
    if (!k.lead_at) return false
    const day = vnDay(k.lead_at)
    const today = vnDay(Date.now())
    if (f.date === 'week') {
      // tuần này: từ thứ Hai
      const now = new Date(today + 'T00:00:00')
      const monday = new Date(now)
      monday.setDate(now.getDate() - ((now.getDay() + 6) % 7))
      if (day < monday.toLocaleDateString('en-CA')) return false
    } else if (f.date === '30d') {
      const d = new Date(today + 'T00:00:00')
      d.setDate(d.getDate() - 29)
      if (day < d.toLocaleDateString('en-CA')) return false
    } else if (f.date === 'custom') {
      if (f.from && day < f.from) return false
      if (f.to && day > f.to) return false
    }
  }
  return true
}

/** Khách hàng: lấy từ pipeline Kinh doanh của Dezon Pro (dezon.tech) */
export const DEZON_PRO = 'https://dezon.tech'
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

