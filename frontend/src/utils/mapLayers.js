// Lớp bản đồ (layout 21/11/2026): hằng số + hook nhớ lớp đang bật. Giao diện ở components/explore/layers.jsx.
import { useState } from 'react'

export const LAYER_META = {
  pending: { label: 'Chưa tới lúc chụp', color: '#F5B301' },
  shoot: { label: 'Cần chụp', color: '#22C55E' },
  dezon: { label: 'Dezon', color: '#1A56DB' },
  // bộ icon 23/11/2026: CTV cam, CTV kinh doanh (SALE) tím, khách hàng đỏ
  ctvUpdate: { label: 'CTV cập nhật', color: '#E8710A' },
  ctvSales: { label: 'CTV kinh doanh', color: '#C026D3' },
  customer: { label: 'Khách hàng', color: '#D93025' },
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


// ---- Bộ lọc tab Công trình và tab Cộng tác viên (góp ý 23/11/2026) ----
const dAgo = (s) => (s ? (Date.now() - new Date(s).getTime()) / 864e5 : Infinity)

/** Tab Công trình: giai đoạn, ngày chụp gần nhất, CTV. */
export const NO_P_FILTER = { stage: '', age: '', ctv: '' }
export const P_AGES = [
  { value: '1d', label: 'Hôm nay, hôm qua' },
  { value: '7d', label: '7 ngày qua' },
  { value: '30d', label: '30 ngày qua' },
  { value: 'old', label: 'Quá 30 ngày' },
]
export function pMatch(p, f) {
  if (f.stage && (p.stage || '').trim() !== (f.stage === '-' ? '' : f.stage)) return false
  if (f.ctv && String(p.ctv_id || '') !== f.ctv) return false
  if (f.age) {
    const d = dAgo(p.last_at || p.at)
    if (f.age === '1d' && d > 2) return false
    if (f.age === '7d' && d > 7) return false
    if (f.age === '30d' && d > 30) return false
    if (f.age === 'old' && d <= 30) return false
  }
  return true
}

/** Tab Cộng tác viên: loại, trạng thái, có công trình chưa, lần gửi gần nhất. */
export const NO_V_FILTER = { type: '', status: '', has: '', last: '' }
export function vMatch(c, f) {
  if (f.type && (c.type === 'sales' ? 'sales' : 'update') !== f.type) return false
  if (f.status && (c.status === 'active' ? 'active' : 'registered') !== f.status) return false
  if (f.has === 'yes' && !(c.count > 0)) return false
  if (f.has === 'no' && c.count > 0) return false
  if (f.last) {
    const d = dAgo(c.last)
    if (f.last === '7d' && d > 7) return false
    if (f.last === '30d' && d > 30) return false
    if (f.last === 'none' && c.last) return false
  }
  return true
}

/** đếm khách theo 1 ô (cả danh sách), xếp nhiều trước; '' gom thành '-' (chưa rõ) */
export function facet(all, get, empty) {
  const m = new Map()
  for (const k of all) {
    const v = get(k) || ''
    m.set(v, (m.get(v) || 0) + 1)
  }
  return [...m.entries()]
    .sort((a, b) => (a[0] === '') - (b[0] === '') || b[1] - a[1] || String(a[0]).localeCompare(String(b[0]), 'vi'))
    .map(([v, n]) => ({ value: v === '' ? '-' : String(v), label: `${v === '' ? empty : v} (${n})` }))
}
