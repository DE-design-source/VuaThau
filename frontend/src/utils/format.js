// Định dạng giống web cũ / app.
export const money = (n) => {
  const v = Math.round(Number(n) || 0)
  return (v < 0 ? '-' : '') + String(Math.abs(v)).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'đ'
}

const VN_OFFSET_MIN = 7 * 60
const pad = (n) => String(n).padStart(2, '0')

/**
 * Tách ngày giờ theo giờ Việt Nam. Nhận RFC 3339 ("2026-11-13T14:13:05+07:00", "...Z") hoặc
 * "YYYY-MM-DD HH:MM[:SS]" / "YYYY-MM-DD" (đã là giờ Việt Nam). Trả {y, mo, d, h, mi} (chuỗi 2 số) hoặc null.
 */
function vnParts(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::\d{2}(?:\.\d+)?)?)?\s*(Z|[+-]\d{2}:?\d{2})?$/i.exec(String(s || '').trim())
  if (!m) return null
  const [, y, mo, d, h, mi, tz] = m
  // Không có múi giờ, hoặc đúng +07:00: dùng thẳng
  if (!tz || /^\+07:?00$/.test(tz)) return { y, mo, d, h: h ?? null, mi: mi ?? null }
  const t = new Date(String(s).trim())
  if (Number.isNaN(t.getTime())) return { y, mo, d, h: h ?? null, mi: mi ?? null }
  const v = new Date(t.getTime() + VN_OFFSET_MIN * 60_000)
  return {
    y: String(v.getUTCFullYear()),
    mo: pad(v.getUTCMonth() + 1),
    d: pad(v.getUTCDate()),
    h: pad(v.getUTCHours()),
    mi: pad(v.getUTCMinutes()),
  }
}

/** "2026-11-13T14:13:05+07:00" hoặc "2026-11-06 12:54" -> "06/11/2026" */
export const vnDate = (s) => {
  const p = vnParts(s)
  return p ? `${p.d}/${p.mo}/${p.y}` : ''
}

/** "2026-11-13T14:13:05+07:00" hoặc "2026-11-06 12:54" -> "12:54 06/11/2026" */
export const vnTime = (s) => {
  const p = vnParts(s)
  if (!p) return s || ''
  return p.h ? `${p.h}:${p.mi} ${p.d}/${p.mo}/${p.y}` : `${p.d}/${p.mo}/${p.y}`
}

/** "20.000", "-20 000đ" -> 20000 / -20000 (API nhận tiền là số nguyên). Không đọc được: NaN. */
export const parseMoney = (s) => {
  const t = String(s ?? '').replace(/[\s.,đ]/gi, '')
  return /^-?\d+$/.test(t) ? Number(t) : NaN
}

export const initials =(name = '?') => {
  const w = String(name).trim().split(/\s+/).filter(Boolean)
  if (!w.length) return '?'
  return (w[0][0] + (w.length > 1 ? w[w.length - 1][0] : '')).toUpperCase()
}

/** "Vietcombank · 0123 · NGUYEN VAN A" -> {name, no, owner} (như bank_parts của máy chủ) */
export const bankParts = (s = '') => {
  const p = String(s || '').split('·').map((x) => x.trim())
  return p.length === 3 ? { name: p[0], no: p[1], owner: p[2] } : { name: s || '', no: '', owner: '' }
}

export const ROLE_LABEL = { ctv: 'Cộng tác viên', admin: 'Admin', ketoan: 'Kế toán', sale: 'Sale' }

// Lý do mẫu khi trả lại / từ chối: dùng khi chưa tải được GET /config (config.reasons)
export const REASONS = {
  tra_lai: ['Ảnh mờ, chụp lại giúp', 'Thiếu ảnh bảng hiệu nhà thầu', 'Sai SĐT nhà thầu', 'Sai địa chỉ công trình'],
  tu_choi: ['Không phải công trình đang xây', 'Trùng nhà đã nộp', 'Ảnh chụp lại màn hình'],
}

export const today = () => new Date().toISOString().slice(0, 10)

/** Link Google Maps của 1 toạ độ: mở được trên máy tính, iPhone, Android (mở app Google Maps nếu có). Không cần key. */
export const gmapUrl = (lat, lng) => `https://www.google.com/maps/search/?api=1&query=${lat},${lng}`

/** Chép link Google Maps để dán vào Zalo, Messenger... */
export const copyGmap = (lat, lng, toast) =>
  navigator.clipboard?.writeText(gmapUrl(lat, lng)).then(
    () => toast?.('Đã chép link Google Maps, dán vào Zalo / Messenger để gửi'),
    () => {},
  )

/** Công trình quá staleDays ngày chưa chụp lại (cần cập nhật). */
export const isStale = (lastAt, staleDays = 30) => !!lastAt && Date.now() - new Date(lastAt).getTime() > staleDays * 864e5
/** Khoảng cách 2 toạ độ (mét). */
export function distM(lat1, lng1, lat2, lng2) {
  const R = 6371000, r = Math.PI / 180
  const a = Math.sin(((lat2 - lat1) * r) / 2) ** 2 + Math.cos(lat1 * r) * Math.cos(lat2 * r) * Math.sin(((lng2 - lng1) * r) / 2) ** 2
  return 2 * R * Math.asin(Math.sqrt(a))
}
/** "350 m" / "1,2 km" */
export const distText = (m) => (m < 1000 ? `${Math.round(m / 10) * 10} m` : `${(m / 1000).toFixed(1).replace('.', ',')} km`)

/** Thoát ký tự HTML (chữ chèn vào popup bản đồ). */
export const esc = (s = '') => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])

/** Mã hồ sơ dạng HS-00081 (giống app). */
export const hsId =(id) => `HS-${String(id).padStart(5, '0')}`

/** Ô thông tin AI đọc trên ảnh (khoá ai_diff của hồ sơ) → nhãn. */
export const AI_LABEL = {
  contractor_name: 'tên nhà thầu',
  contractor_phone: 'SĐT nhà thầu',
  floors: 'số tầng',
  stage: 'giai đoạn',
}

/** "tên nhà thầu, SĐT nhà thầu" từ ai_diff; '' nếu không lệch. */
export const aiDiffText = (diff) =>
  Object.keys(diff || {})
    .map((k) => AI_LABEL[k] || k)
    .join(', ')
