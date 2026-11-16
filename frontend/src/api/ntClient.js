// Gọi máy chủ Nghiệm thu (nghiemthu.vuathau.com.vn), CÙNG máy chủ app CTV đang dùng, nên web và app chung dữ liệu.
// Gửi header X-App: 1 thì mọi route trả JSON: GET trả dữ liệu trang, POST trả {url, msgs, token}.
// Mặc định gọi qua đường dẫn /nt trên cùng tên miền (Vite proxy khi dev, Caddy khi chạy thật) để khỏi vướng CORS.
import storageService from '@/services/storageService'

export const NT_BASE = (import.meta.env.VITE_NT_URL || '/nt').replace(/\/$/, '')

let token = storageService.getToken()
let onSessionEnd = null
let onNewToken = null

/** Đăng ký hàm chạy khi hết phiên / bị khoá (store dispatch logout). */
export function setSessionHandlers({ end, renew }) {
  onSessionEnd = end
  onNewToken = renew
}

export function setNtToken(t) {
  token = t || null
}

export const authHeaders = () => ({ 'X-App': '1', ...(token ? { Authorization: `Bearer ${token}` } : {}) })

async function handle(res, path) {
  const j = await res.json().catch(() => null) // null: không phải JSON
  if (!j || typeof j !== 'object') throw new Error(`Máy chủ đang lỗi (mã ${res.status}). Thử lại sau ít phút.`)
  // Đổi mật khẩu: máy chủ trả token mới
  if (j.token && token && j.token !== token && !path.startsWith('/login')) onNewToken?.(j.token)
  const locked = res.status === 403 && String(j.error || '').includes('đã bị khoá')
  if ((j.url === '/login' || locked) && !path.startsWith('/login')) {
    const msg = j.error || j.msgs?.[0] || 'Hết phiên đăng nhập, đăng nhập lại nhé.'
    if (token) onSessionEnd?.(msg)
    throw new Error(msg)
  }
  if (j.error) throw new Error(j.error)
  if (res.status >= 400) throw new Error(`Có lỗi xảy ra (mã ${res.status}).`)
  return j
}

async function send(path, init = {}) {
  let res
  try {
    res = await fetch(NT_BASE + path, { ...init, headers: { ...authHeaders(), ...(init.headers || {}) } })
  } catch {
    throw new Error('Mất kết nối tới máy chủ. Kiểm tra mạng rồi thử lại.')
  }
  return handle(res, path)
}

/** {a: 1, ids: [1, 2]} -> "?a=1&ids=1&ids=2" (bỏ giá trị rỗng) */
export function qs(params = {}) {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (Array.isArray(v)) v.forEach((x) => p.append(k, x))
    else if (v !== '' && v !== null && v !== undefined && v !== 0) p.append(k, v)
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

function formBody(body) {
  if (body instanceof FormData) return body
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(body || {})) {
    if (Array.isArray(v)) v.forEach((x) => p.append(k, x))
    else if (v !== undefined && v !== null) p.append(k, v)
  }
  return p
}

const ntClient = {
  get: (path) => send(path),
  /** POST form (object hoặc FormData). Trả {url, msgs, token?}. */
  post: (path, body = {}) => send(path, { method: 'POST', body: formBody(body) }),

  /** Tải file cần đăng nhập (ảnh, Excel): trả Blob. */
  async blob(path) {
    let res
    try {
      res = await fetch(NT_BASE + path, { headers: authHeaders() })
    } catch {
      throw new Error('Mất kết nối tới máy chủ.')
    }
    if (!res.ok || (res.headers.get('content-type') || '').includes('application/json')) {
      await handle(res, path) // ném lỗi kèm câu báo của máy chủ
      throw new Error(`Không tải được (mã ${res.status}).`)
    }
    return res.blob()
  },

  /** Tải file về máy (Excel, CSV). */
  async download(path, filename) {
    const b = await this.blob(path)
    const url = URL.createObjectURL(b)
    const a = Object.assign(document.createElement('a'), { href: url, download: filename })
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(url), 10_000)
  },
}

export default ntClient
