const toDate = (value) => (value instanceof Date ? value : new Date(value))

/** 15/11/2026, 14:30 */
export function formatDateTime(value, locale = 'vi-VN') {
  const date = toDate(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(date)
}

/** 15/11/2026 */
export function formatDate(value, locale = 'vi-VN') {
  const date = toDate(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(locale, { dateStyle: 'short' }).format(date)
}

/** "5 phút trước", "2 ngày trước"... */
export function formatRelative(value, locale = 'vi-VN') {
  const date = toDate(value)
  if (Number.isNaN(date.getTime())) return ''
  const seconds = Math.round((date.getTime() - Date.now()) / 1000)
  const units = [
    ['year', 31_536_000],
    ['month', 2_592_000],
    ['day', 86_400],
    ['hour', 3_600],
    ['minute', 60],
  ]
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit)
  }
  return rtf.format(seconds, 'second')
}

export default formatDate
