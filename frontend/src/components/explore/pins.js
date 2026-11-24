// Bộ icon bản đồ (góp ý 23-24/11/2026): ghim tròn có chữ, đuôi nhọn chỉ đúng vị trí, tên hiện cạnh ghim.
//   KH: chữ trắng nền đỏ #D93025 · CTV tích cực: chữ trắng nền cam #E8710A
//   CTV đăng ký: chữ cam nền trắng viền cam · SALE (CTV kinh doanh): chữ trắng nền tím #C026D3
// Dùng chung cho ghim trên bản đồ (SiteMap) và chú thích bảng Lớp bản đồ (layers.jsx).

export const MK = { kh: '#D93025', ctv: '#E8710A', sale: '#C026D3' }

/** SVG ghim tròn có chữ (text: KH / CTV / SALE; outline: nền trắng viền màu). w: rộng (px), cao tự theo tỉ lệ. */
export function badgePin(text, c, outline = false, w = 30) {
  const long = text.length > 3
  return (
    `<svg viewBox="0 0 40 52" width="${w}" height="${Math.round((w * 52) / 40)}" aria-hidden="true">` +
    `<path d="M20 50 12.5 34h15Z" fill="${c}"/>` +
    `<circle cx="20" cy="19" r="16.5" fill="${outline ? '#fff' : c}" stroke="${outline ? c : '#fff'}" stroke-width="${outline ? 3.6 : 3.2}"/>` +
    `<text x="20" y="${long ? 22.6 : 23.6}" text-anchor="middle" font-size="${long ? 10.5 : 12.5}" font-weight="800" letter-spacing="-.2" fill="${outline ? c : '#fff'}">${text}</text></svg>`
  )
}

/** Ghim của 1 cộng tác viên: SALE tím (kinh doanh), CTV cam đặc (tích cực) / viền cam (đã đăng ký). */
export const ctvBadge = (c, w) => (c.type === 'sales' ? badgePin('SALE', MK.sale, false, w) : badgePin('CTV', MK.ctv, c.status !== 'active', w))

/** Ghim khách hàng: KH đỏ. */
export const cusBadge = (w) => badgePin('KH', MK.kh, false, w)
