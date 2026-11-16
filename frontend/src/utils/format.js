// Định dạng giống web cũ / app.
export const money = (n) => {
  const v = Math.round(Number(n) || 0)
  return (v < 0 ? '-' : '') + String(Math.abs(v)).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'đ'
}

/** "2026-11-06 12:54:53" -> "06/11/2026" */
export const vnDate = (s) => (s && s.length >= 10 ? `${s.slice(8, 10)}/${s.slice(5, 7)}/${s.slice(0, 4)}` : '')

/** "2026-11-06 12:54:53" -> "12:54 06/11/2026" */
export const vnTime = (s) => (s && s.length >= 16 ? `${s.slice(11, 16)} ${vnDate(s)}` : s || '')

export const initials = (name = '?') => {
  const w = String(name).trim().split(/\s+/).filter(Boolean)
  if (!w.length) return '?'
  return (w[0][0] + (w.length > 1 ? w[w.length - 1][0] : '')).toUpperCase()
}

/** "Vietcombank · 0123 · NGUYEN VAN A" -> {name, no, owner} (như bank_parts của máy chủ) */
export const bankParts = (s = '') => {
  const p = String(s || '').split('·').map((x) => x.trim())
  return p.length === 3 ? { name: p[0], no: p[1], owner: p[2] } : { name: s || '', no: '', owner: '' }
}

export const ROLE_LABEL = { ctv: 'Cộng tác viên', admin: 'Admin', ketoan: 'Kế toán' }

// Lý do mẫu khi trả lại / từ chối (giống REASONS của máy chủ)
export const REASONS = {
  tra_lai: ['Ảnh mờ, chụp lại giúp', 'Thiếu ảnh bảng hiệu nhà thầu', 'Sai SĐT nhà thầu', 'Sai địa chỉ công trình'],
  tu_choi: ['Không phải công trình đang xây', 'Trùng nhà đã nộp', 'Ảnh chụp lại màn hình'],
}

export const today = () => new Date().toISOString().slice(0, 10)
