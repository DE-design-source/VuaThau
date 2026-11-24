// Chế độ ẩn thông tin CTV và nhà thầu (góp ý 25/11/2026): nút gạt trên thanh trên (admin, kế toán, sale), dùng khi
// chiếu màn hình / cho khách xem. Che ngay lúc nhận dữ liệu từ máy chủ (axiosClient), nên mọi trang, thẻ, popup bản đồ,
// danh sách, ô lọc đều ẩn theo, không phải sửa từng chỗ hiển thị:
//   - tên CTV -> "CTV #<id>"; tên / công ty nhà thầu -> "Nhà thầu NT-XXXX" (mã cố định theo tên: lọc, sắp xếp vẫn đúng);
//   - SĐT, tài khoản ngân hàng, email, người liên hệ: che;
//   - ảnh công trình: che SĐT và mọi dòng chữ trên ảnh (?hide=1; ảnh chưa quét chữ: làm mờ cả ảnh).
// Thông tin khách hàng (tab Khách hàng) giữ nguyên. Trình duyệt nhớ lựa chọn.

const KEY = 'vuathau.hidePII'

export function privacyOn() {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

/** Bật / tắt rồi tải lại trang (dữ liệu đang hiện được lấy lại, che hoặc bỏ che). */
export function setPrivacy(on) {
  try {
    if (on) localStorage.setItem(KEY, '1')
    else localStorage.removeItem(KEY)
  } catch {
    /* trình duyệt chặn lưu: không bật được */
  }
  window.location.reload()
}

const PHONE = '•••• ••• •••'

/** Mã cố định của 1 tên nhà thầu (cùng tên -> cùng mã) */
function code(s) {
  let h = 2166136261
  for (const ch of String(s).normalize('NFC').toLowerCase().replace(/\s+/g, ' ').trim()) {
    h ^= ch.codePointAt(0)
    h = Math.imul(h, 16777619)
  }
  return (h >>> 0).toString(16).toUpperCase().padStart(8, '0').slice(0, 4)
}

const ntAlias = (s) => (s && String(s).trim() ? `Nhà thầu NT-${code(s)}` : s)
const ctvAlias = (id) => (id ? `CTV #${id}` : 'CTV (đã ẩn)')
const hidePhone = (s) => (s ? PHONE : s)
const hideText = (s) => (s ? '(đã ẩn)' : s)

/** Che chuỗi SĐT lẫn trong chữ (ghi chú, mô tả...): 9-11 chữ số, có thể cách bằng dấu chấm / cách / gạch */
const phoneInText = /(?:\+?84|0)(?:[\s.-]?\d){8,10}/g
const hideInText = (s) => (typeof s === 'string' ? s.replace(phoneInText, PHONE) : s)

// Khoá luôn che, ở bất kỳ đâu trong dữ liệu
const ALWAYS = {
  ctv_phone: hidePhone,
  contractor_phone: hidePhone,
  ct_phone: hidePhone,
  login_phone: hidePhone,
  ctv_bank: hideText,
  user_bank: hideText,
  bank_no: hideText,
  bank_owner: hideText,
  contact: hideText,
  contractor: (v) => (typeof v === 'string' ? ntAlias(v) : v),
  contractor_name: ntAlias,
  ct_name: ntAlias,
  company: ntAlias,
  brand: ntAlias,
  contractor_url: () => '',
  alt_phones: (v) => (Array.isArray(v) ? v.map(() => PHONE) : v),
  note: hideInText,
}

/** Đối tượng là 1 người (CTV / nhà thầu): che tên, tên đăng nhập (SĐT), SĐT, email, ngân hàng. */
function maskPerson(o, kind) {
  const id = o.ctv_id ?? o.user_id ?? o.id // đánh giá app: id là của lượt đánh giá, user_id mới là CTV
  if ('name' in o) o.name = kind === 'nt' ? ntAlias(o.name) : ctvAlias(id)
  for (const k of ['username', 'phone']) if (k in o) o[k] = hidePhone(o[k])
  if ('email' in o) o.email = hideText(o.email)
  if ('bank' in o) o.bank = typeof o.bank === 'string' ? hideText(o.bank) : o.bank && typeof o.bank === 'object' ? {} : o.bank
}

/**
 * Đối tượng trong dữ liệu là CTV / nhà thầu? Theo vai trò (users), theo khoá cha (ctv_pins, ctvs...),
 * hoặc theo trang đang gọi (danh sách CTV, nhà thầu, phiếu chi, đánh giá app...).
 */
function personKind(o, parentKey, url) {
  if (o.role === 'ctv') return 'ctv'
  if (o.role === 'nhathau') return 'nt'
  if (['ctv_pins', 'ctvs', 'ctv'].includes(parentKey)) return 'ctv'
  if (['contractors', 'nt'].includes(parentKey)) return 'nt'
  // theo trang: chỉ đối tượng giống 1 người (có SĐT / tên đăng nhập / ngân hàng), không đụng tỉnh, loại công trình...
  if (!('phone' in o) && !('username' in o) && !('bank' in o)) return ''
  if (/^\/(contractors|sale\/contractors|sale\/dezon-contractors|admin\/nt)(\/|\?|$)/.test(url)) return 'nt'
  if (/^\/(ctvs|batches|accounting|payouts|reports|admin\/feedback)(\/|\?|$)/.test(url)) {
    // phiếu chi, kế toán, báo cáo: dòng của CTV có ctv_id / username; người lập phiếu (by_name) giữ nguyên
    return 'ctv'
  }
  return ''
}

function walk(v, parentKey, url) {
  if (parentKey === 'customers') return // khách hàng: giữ nguyên
  if (Array.isArray(v)) {
    v.forEach((x) => walk(x, parentKey, url))
    return
  }
  if (!v || typeof v !== 'object') return
  for (const k of Object.keys(v)) {
    if (k in ALWAYS && (typeof v[k] === 'string' || Array.isArray(v[k]))) v[k] = ALWAYS[k](v[k])
  }
  if ('ctv_name' in v) v.ctv_name = ctvAlias(v.ctv_id)
  if ('ctv_label' in v) v.ctv_label = ctvAlias(v.ctv_id)
  if ('user_name' in v) v.user_name = ctvAlias(v.user_id)
  const kind = personKind(v, parentKey, url)
  if (kind) maskPerson(v, kind)
  for (const k of Object.keys(v)) {
    if (v[k] && typeof v[k] === 'object') walk(v[k], k, url)
  }
}

/** Che dữ liệu `data` của 1 response (url tương đối /api/v1, vd "/ctvs/12"). Sửa tại chỗ. */
export function maskResponse(data, url) {
  // khách hàng (Dezon Pro), cấu hình, tài khoản đang đăng nhập: không đụng
  if (/^\/(customers|config|me|users\/me|settings|staff)(\/|\?|$)/.test(url)) {
    return data
  }
  walk(data, '', url)
  return data
}

/** Ảnh công trình khi đang ẩn: máy chủ che SĐT + mọi dòng chữ trên ảnh (bảng hiệu, băng rôn, dải chữ in trên ảnh);
 *  ảnh chưa quét chữ thì làm mờ cả ảnh. */
export function photoParams(url, params) {
  if (!privacyOn() || !/^\/photos\/\d+/.test(url)) return params
  return { ...(params || {}), hide: 1 }
}
