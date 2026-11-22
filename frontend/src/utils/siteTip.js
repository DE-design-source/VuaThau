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

/** p: điểm trên bản đồ; o: { staleDays, prov (tên tỉnh), status (nhãn trạng thái, chỉ admin), ctvLink (tên CTV bấm mở hồ sơ CTV: admin, kế toán) } */
export function siteTip(p, { staleDays = 30, prov = '', status = '', ctvLink = false } = {}) {
  const last = p.last_at || p.at
  const stale = isStale(last, staleDays)
  const row = (label, v) => (v ? `<div class="site-tip-row"><span>${label}</span>${esc(v)}</div>` : '')
  return (
    `<div class="site-tip">` +
    (p.cover_url
      ? `<div class="site-tip-img"><img src="${esc(p.cover_url)}" alt="" onerror="this.parentNode.remove()"></div>`
      : p.cover_id
        ? `<div class="site-tip-img"><img data-cover="${p.cover_id}" alt=""></div>`
        : '') +
    `<div class="site-tip-body">` +
    `<div class="site-tip-top"><span class="mono">${hsId(p.id)}</span>${p.vip ? ' <span class="vip-tag">VIP</span>' : ''}` +
    `${status ? ` · ${esc(status)}` : ''}</div>` +
    `<b class="site-tip-addr">${esc(p.address || '(chưa có địa chỉ)')}</b>` +
    row('Tỉnh', prov) +
    row('Nhà thầu', p.contractor) +
    row('Giai đoạn', p.stage) +
    (ctvLink && p.ctv_id && p.ctv_name
      ? `<div class="site-tip-row"><span>CTV</span><a class="site-tip-ctv" href="/ctvs/${p.ctv_id}" data-to="/ctvs/${p.ctv_id}">${esc(p.ctv_name)}</a></div>`
      : row('CTV', p.ctv_name)) +
    `<div class="site-tip-when ${stale ? 'stale' : ''}">${stale ? '<b>Cần cập nhật công trình</b> · ' : ''}chụp lần cuối ${esc(vnDate(last))}</div>` +
    `</div></div>`
  )
}

/** Thẻ của 1 khách hàng trên bản đồ (lớp Khách hàng: pipeline Kinh doanh Dezon Pro, admin / kế toán). */
export function cusTip(k) {
  const tel = (k.phone || '').replace(/[^\d+]/g, '')
  const addr = [k.address, k.area].filter(Boolean).join(', ')
  const value = k.value_vnd > 0 ? (k.value_vnd >= 1e9 ? `${(k.value_vnd / 1e9).toLocaleString('vi-VN', { maximumFractionDigits: 2 })} tỷ` : `${Math.round(k.value_vnd / 1e6).toLocaleString('vi-VN')} triệu`) : ''
  return (
    `<div class="site-tip"><div class="site-tip-body">` +
    `<div class="site-tip-top"><b class="cus-tip-name">${esc(k.name || 'Khách hàng')}</b>${k.kind ? ` <span class="cus-tag">${esc(k.kind)}</span>` : ''}</div>` +
    (addr ? `<div class="site-tip-row"><span>${k.address ? 'Địa chỉ' : 'Khu vực'}</span>${esc(addr)}${k.address ? '' : ' <i class="muted">(ghim ở trung tâm khu vực)</i>'}</div>` : '') +
    (/^\+?\d{9,12}$/.test(tel) ? `<div class="site-tip-row"><span>SĐT</span><a href="tel:${tel}">${esc(k.phone)}</a></div>` : '') +
    `<div class="site-tip-row"><span>Phụ trách</span><b>${esc(k.owner || '—')}</b></div>` +
    (k.stage ? `<div class="site-tip-row"><span>Giai đoạn</span>${esc(k.stage)}</div>` : '') +
    (k.scale || value ? `<div class="site-tip-row"><span>Quy mô</span>${esc([k.scale, value].filter(Boolean).join(' · '))}</div>` : '') +
    (k.source ? `<div class="site-tip-row"><span>Nguồn</span>${esc(k.source)}</div>` : '') +
    (k.rating > 0 ? `<div class="site-tip-row"><span>Giao tiếp</span><span class="cus-stars">${'★'.repeat(k.rating)}</span></div>` : '') +
    `</div></div>` +
    `<div class="site-pop-actions"><a href="https://dezon.tech/kinh-doanh?lead=${encodeURIComponent(k.ext_id)}" target="_blank" rel="noreferrer">Mở trên Dezon Pro</a></div>`
  )
}

/** Thẻ của 1 cộng tác viên trên bản đồ (lớp Cộng tác viên, admin / kế toán). */
export function ctvTip(c) {
  const type = c.type === 'sales' ? 'CTV kinh doanh' : 'CTV cập nhật'
  const st = c.status === 'active' ? 'Tích cực' : 'Đã đăng ký'
  const tel = (c.phone || '').replace(/[^\d+]/g, '')
  return (
    `<div class="site-tip ctv-tip"><div class="site-tip-body">` +
    `<div class="site-tip-top"><span class="ctv-tip-type ${c.type === 'sales' ? 'sales' : ''}">${type}</span> · ${st}</div>` +
    `<b class="site-tip-addr">${esc(c.name)}</b>` +
    (c.address ? `<div class="site-tip-row"><span>Địa chỉ</span>${esc(c.address)}${c.approx ? ' <i class="muted">(vị trí gần đúng)</i>' : ''}</div>` : '') +
    (/^\+?\d{9,12}$/.test(tel) ? `<div class="site-tip-row"><span>SĐT</span><a href="tel:${tel}">${esc(c.phone)}</a></div>` : '') +
    `<div class="site-tip-row"><span>Đã gửi</span>${c.count} công trình${c.last ? ` · lần cuối ${esc(vnDate(c.last))}` : ''}</div>` +
    `</div></div>` +
    `<div class="site-pop-actions"><a href="/ctvs/${c.id}" data-to="/ctvs/${c.id}">Hồ sơ CTV</a>` +
    (c.count ? `<a href="/map?ctv=${c.id}" data-to="/map?ctv=${c.id}">Công trình của CTV</a>` : '') +
    `</div>`
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
