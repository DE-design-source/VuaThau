// Thẻ hiện khi rê chuột vào 1 công trình trên bản đồ: hình đại diện (ảnh lần chụp gần nhất) + thông tin chính.
// Ảnh cần đăng nhập nên thẻ chỉ có chỗ trống <img data-cover>, SiteMap tải ảnh (kèm token) khi thẻ mở.
import { getBlob } from '@/api/axiosClient'
import { esc, hsId, isStale, vnDate } from '@/utils/format'

/** Tách địa chỉ thành 2 dòng thẻ: [số nhà, đường…] và [phường/xã, tỉnh]. prov: tên tỉnh khi địa chỉ quá ngắn. */
export function splitAddr(addr, prov = '') {
  const parts = (addr || '').split(',').map((x) => x.trim()).filter(Boolean)
  if (!parts.length) return ['(chưa có địa chỉ)', prov]
  if (parts.length === 1) return [parts[0], prov]
  const n = parts.length >= 3 ? 2 : 1
  return [parts.slice(0, -n).join(', '), parts.slice(-n).join(', ')]
}

/** Props SiteCard của 1 công trình. o: { to, prov, staleDays, fresh (Set id mới) } */
export function cardProps(p, { to, prov = '', staleDays = 30, fresh } = {}) {
  const [street, area] = splitAddr(p.address, prov)
  return {
    to,
    tag: (fresh?.has(p.id) ? 'Mới · ' : '') + ((p.stage || '').trim() || 'Công trình'),
    street,
    area,
    contractor: p.contractor,
    ctv: p.ctv_name,
    date: vnDate(p.last_at || p.at),
    stale: isStale(p.last_at || p.at, staleDays),
  }
}

/** p: điểm trên bản đồ; o: { staleDays, prov (tên tỉnh), status (nhãn trạng thái, chỉ admin) } */
export function siteTip(p, { staleDays = 30, prov = '', status = '' } = {}) {
  const last = p.last_at || p.at
  const stale = isStale(last, staleDays)
  const row = (label, v) => (v ? `<div class="site-tip-row"><span>${label}</span>${esc(v)}</div>` : '')
  return (
    `<div class="site-tip">` +
    (p.cover_id ? `<div class="site-tip-img"><img data-cover="${p.cover_id}" alt=""></div>` : '') +
    `<div class="site-tip-body">` +
    `<div class="site-tip-top"><span class="mono">${hsId(p.id)}</span>${p.vip ? ' <span class="vip-tag">VIP</span>' : ''}` +
    `${status ? ` · ${esc(status)}` : ''}</div>` +
    `<b class="site-tip-addr">${esc(p.address || '(chưa có địa chỉ)')}</b>` +
    row('Tỉnh', prov) +
    row('Nhà thầu', p.contractor) +
    row('Giai đoạn', p.stage) +
    row('CTV', p.ctv_name) +
    `<div class="site-tip-when ${stale ? 'stale' : ''}">${stale ? '<b>Cần cập nhật công trình</b> · ' : ''}chụp lần cuối ${esc(vnDate(last))}</div>` +
    `</div></div>`
  )
}

/** Thẻ của 1 công trình Dezon (nhà thầu đăng trên dezon.vn). Ảnh là link công khai của dezon.vn. */
export function dezonTip(p, { prov = '' } = {}) {
  const row = (label, v) => (v ? `<div class="site-tip-row"><span>${label}</span>${esc(v)}</div>` : '')
  const link = (href, text) => (href ? `<a href="${esc(href)}" target="_blank" rel="noreferrer">${text}</a>` : '')
  return (
    `<div class="site-tip">` +
    (p.cover ? `<div class="site-tip-img"><img src="${esc(p.cover)}" alt="" referrerpolicy="no-referrer" loading="lazy" onerror="this.parentNode.remove()"></div>` : '') +
    `<div class="site-tip-body">` +
    `<div class="site-tip-top"><span class="dz-tag">Dezon</span>${p.geo_level === 'khu_vuc' ? ' · vị trí gần đúng (theo phường, xã)' : ''}</div>` +
    `<b class="site-tip-addr">${esc(p.title)}</b>` +
    row('Tỉnh', prov) +
    row('Nhà thầu', p.contractor) +
    (p.posted_at ? `<div class="site-tip-when">đăng trên dezon.vn ${esc(vnDate(p.posted_at))}</div>` : '') +
    `</div></div>` +
    `<div class="site-pop-actions">${link(p.url, 'Xem trên Dezon')}${link(p.contractor_url, 'Nhà thầu')}` +
    `<a href="https://www.google.com/maps/search/?api=1&query=${p.lat},${p.lng}" target="_blank" rel="noreferrer">Google Maps</a></div>`
  )
}

// Ảnh đại diện đã tải (id ảnh -> URL blob), dùng lại khi rê chuột lần sau
const covers = new Map()

/** Điền ảnh vào thẻ vừa mở (el: phần tử của tooltip). */
export function fillCover(el, id) {
  const img = el?.querySelector(`img[data-cover="${id}"]`)
  if (!img) return
  if (!covers.has(id)) {
    covers.set(id, getBlob(`/photos/${id}`).then((b) => URL.createObjectURL(b), () => null))
  }
  covers.get(id).then((url) => {
    if (url) img.src = url
    else img.closest('.site-tip-img')?.remove()
  })
}

/** Chữ ngắn trên nhãn bản đồ / tiêu đề thẻ: giai đoạn thi công. */
export function stageLabel(stage) {
  const s = (stage || '').trim()
  if (!s) return 'Công trình'
  // 8 giai đoạn (18/11/2026) + tên cũ (hồ sơ trước đó)
  const short = {
    'Chuẩn bị mặt bằng': 'Mặt bằng',
    'Ép cọc, đào móng': 'Ép cọc',
    'Thi công móng': 'Làm móng',
    'Xây phần thô': 'Phần thô',
    'Xây tường, đổ mái': 'Tường, mái',
    'Điện nước âm tường': 'Điện nước',
    'Đào móng / làm móng': 'Làm móng',
    'Đã hoàn công': 'Bàn giao',
  }[s]
  if (short) return short
  return s.length > 16 ? s.slice(0, 15) + '…' : s
}

/** Props SiteCard của 1 công trình Dezon (nhà thầu đăng trên dezon.vn, chưa CTV nào chụp). */
export function dezonCard(p, { prov = '' } = {}) {
  const [street, area] = splitAddr(p.title, prov)
  return {
    id: `dz${p.id}`,
    href: p.url || undefined,
    img: p.cover || undefined,
    dz: true,
    tag: 'Dezon · chưa ai chụp',
    street,
    area,
    contractor: p.contractor,
    source: 'Nhà thầu đăng trên Dezon',
    date: vnDate(p.posted_at),
  }
}
