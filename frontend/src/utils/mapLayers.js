// Lớp bản đồ (layout 21/11/2026): hằng số + hook nhớ lớp đang bật. Giao diện ở components/explore/layers.jsx.
import { useState } from 'react'
import { isStale } from '@/utils/format'

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
export const NO_CUS_FILTER = { owner: '', province: '', source: '', rating: '', date: '', from: '', to: '', sort: '' }

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

/** Bỏ dấu, chữ thường (tìm nhanh, so tên tỉnh) */
export const fold = (s) => (s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[đĐ]/g, 'd').toLowerCase()

/** Tab Công trình: giai đoạn, lần chụp (chưa tới lúc / cần chụp, như chú thích), ngày chụp gần nhất, CTV.
 *  Tỉnh thành dùng chung ô Tỉnh thành của thanh tìm (lọc ở máy chủ). */
export const NO_P_FILTER = { stage: '', shot: '', age: '', ctv: '', nt: '', sort: '' }
export const P_AGES = [
  { value: '1d', label: 'Hôm nay, hôm qua' },
  { value: '7d', label: '7 ngày qua' },
  { value: '30d', label: '30 ngày qua' },
  { value: 'old', label: 'Quá 30 ngày' },
]
/** Tên nhà thầu của công trình (ô Nhà thầu, góp ý 25/11/2026): bỏ khoảng trắng thừa */
export const ntName = (p) => (p.contractor || '').replace(/\s+/g, ' ').trim()

// ---- Ô Sắp xếp (góp ý 25/11/2026: "sắp xếp từ cao xuống thấp" ở cả 3 tab). '' = mặc định ----
const time = (s) => (s ? new Date(s).getTime() : 0)
/** Tab Công trình: mặc định mới chụp nhất trước */
export const P_SORTS = [
  { value: 'old', label: 'Lâu chưa chụp nhất' },
  { value: 'nt', label: 'Nhà thầu nhiều công trình nhất' },
  { value: 'ctv', label: 'CTV nhiều công trình nhất' },
]
/** items: [{ p, at }] (hồ sơ CTV + Dezon). Trả mảng mới đã sắp xếp. */
export function sortProjects(items, sort) {
  const byNew = (a, b) => time(b.at) - time(a.at)
  if (sort === 'old') return [...items].sort((a, b) => time(a.at) - time(b.at))
  if (sort === 'nt' || sort === 'ctv') {
    const key = (it) => (sort === 'nt' ? ntName(it.p) : it.d ? '' : String(it.p.ctv_id || ''))
    const n = new Map()
    for (const it of items) n.set(key(it), (n.get(key(it)) || 0) + 1)
    // nhóm nhiều công trình lên trước (chưa rõ tên xuống cuối), trong nhóm: mới nhất trước
    return [...items].sort((a, b) => (key(a) === '') - (key(b) === '') || n.get(key(b)) - n.get(key(a)) || key(a).localeCompare(key(b), 'vi') || byNew(a, b))
  }
  return [...items].sort(byNew)
}
/** Tab Cộng tác viên: mặc định theo máy chủ */
export const V_SORTS = [
  { value: 'count', label: 'Nhiều công trình nhất' },
  { value: 'last', label: 'Gửi gần đây nhất' },
]
export function sortCtvs(ctvs, sort) {
  if (sort === 'count') return [...ctvs].sort((a, b) => (b.count || 0) - (a.count || 0) || time(b.last) - time(a.last))
  if (sort === 'last') return [...ctvs].sort((a, b) => time(b.last) - time(a.last) || (b.count || 0) - (a.count || 0))
  return ctvs
}
/** Tab Khách hàng: mặc định theo máy chủ */
export const CUS_SORTS = [
  { value: 'value', label: 'Giá trị cao nhất' },
  { value: 'rating', label: 'Giao tiếp tốt nhất' },
  { value: 'new', label: 'Mới tạo nhất' },
]
export function sortCustomers(rows, sort) {
  if (sort === 'value') return [...rows].sort((a, b) => (b.value_vnd || 0) - (a.value_vnd || 0))
  if (sort === 'rating') return [...rows].sort((a, b) => (b.rating || 0) - (a.rating || 0) || (b.value_vnd || 0) - (a.value_vnd || 0))
  if (sort === 'new') return [...rows].sort((a, b) => time(b.lead_at) - time(a.lead_at))
  return rows
}

/** staleDays: số ngày tới lúc chụp lại (ô Lần chụp) */
export function pMatch(p, f, staleDays = 30) {
  if (f.stage && (p.stage || '').trim() !== (f.stage === '-' ? '' : f.stage)) return false
  if (f.ctv && String(p.ctv_id || '') !== f.ctv) return false
  if (f.nt && ntName(p) !== (f.nt === '-' ? '' : f.nt)) return false
  if (f.shot && (isStale(p.last_at || p.at, staleDays) ? 'shoot' : 'pending') !== f.shot) return false
  if (f.age) {
    const d = dAgo(p.last_at || p.at)
    if (f.age === '1d' && d > 2) return false
    if (f.age === '7d' && d > 7) return false
    if (f.age === '30d' && d > 30) return false
    if (f.age === 'old' && d <= 30) return false
  }
  return true
}

/** Tab Cộng tác viên: tỉnh thành, loại, trạng thái, có công trình chưa, lần gửi gần nhất.
 *  prov: mã tỉnh của CTV (xem ctvProvinces), '' chưa rõ. */
export const NO_V_FILTER = { prov: '', type: '', status: '', has: '', last: '', sort: '' }
export function vMatch(c, f, prov = '') {
  if (f.prov && prov !== (f.prov === '-' ? '' : f.prov)) return false
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

/**
 * Tỉnh của từng CTV (hồ sơ CTV chưa có ô tỉnh): tên tỉnh có trong địa chỉ (lấy tên nằm sau cùng),
 * không có thì tỉnh CTV đó chụp nhiều công trình nhất. Trả Map id CTV -> mã tỉnh ('' chưa rõ).
 */
export function ctvProvinces(ctvs, provinces, points) {
  const names = (provinces || [])
    .map((p) => [p.code, fold(p.name).replace(/^(thanh pho|tp\.?|tinh)\s+/, '').trim()])
    .filter(([, n]) => n)
    .map(([code, n]) => [code, new RegExp('(^|[^a-z])' + n.replace(/[^a-z0-9 ]/g, '.') + '($|[^a-z])', 'g')])
  const most = new Map() // id CTV -> Map(mã tỉnh -> số công trình)
  for (const p of points || []) {
    if (!p.ctv_id || !p.province) continue
    const m = most.get(p.ctv_id) || new Map()
    m.set(p.province, (m.get(p.province) || 0) + 1)
    most.set(p.ctv_id, m)
  }
  const out = new Map()
  for (const c of ctvs || []) {
    const a = fold(c.address)
    let best = ''
    let at = -1
    for (const [code, re] of names) {
      for (const m of a.matchAll(re)) if (m.index > at) [at, best] = [m.index, code]
    }
    if (!best && most.has(c.id)) best = [...most.get(c.id)].sort((x, y) => y[1] - x[1])[0][0]
    out.set(c.id, best)
  }
  return out
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
