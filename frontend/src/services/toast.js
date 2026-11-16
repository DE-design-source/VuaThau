// Thông báo ngắn góc màn hình (thay thông báo flash của web cũ). Gọi toast() ở đâu cũng được.
const subs = new Set()
let msgs = []
let timer = null

export function toast(m) {
  const list = (Array.isArray(m) ? m : [m]).filter(Boolean).map(String)
  if (!list.length) return
  msgs = list
  subs.forEach((f) => f(msgs))
  clearTimeout(timer)
  timer = setTimeout(() => clearToast(), 3500 + 1500 * list.length)
}

export function clearToast() {
  msgs = []
  subs.forEach((f) => f(msgs))
}

export function subscribeToast(f) {
  subs.add(f)
  return () => subs.delete(f)
}

export const currentToast = () => msgs
