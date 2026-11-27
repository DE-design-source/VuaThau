// Cuộn chuột phóng to/thu nhỏ đúng 1 mức mỗi lần (góp ý 27/11/2026: "mỗi lần cuộn chuột là nó zoom tới 5 lần").
// Bộ cuộn mặc định của Leaflet cộng dồn deltaY: chuột Windows/chuột cuộn nhanh/touchpad (cuộn quán tính) bắn nhiều
// sự kiện liên tiếp nên nhảy 3–5 mức. Ở đây: mỗi "cú cuộn" chỉ 1 mức; cú cuộn kết thúc khi ngừng sự kiện wheel
// GAP ms, hoặc cuộn liên tục thì cứ STEP ms mới cho thêm 1 mức (vẫn cuộn xa được, chỉ không vọt).
const GAP = 180
const STEP = 350
const MIN_DELTA = 4 // touchpad rung nhẹ thì bỏ qua

/** Gắn vào map Leaflet (gọi ngay sau L.map). Trả về hàm gỡ. */
export function stepWheelZoom(m) {
  m.scrollWheelZoom.disable()
  const box = m.getContainer()
  let last = 0 // lần cuối nhận sự kiện wheel
  let fired = 0 // lần cuối đã đổi mức
  const onWheel = (e) => {
    // bảng "Lớp bản đồ", nút +/-: để chúng tự cuộn danh sách
    if (e.target.closest?.('.lp, .leaflet-control')) return
    e.preventDefault()
    const now = Date.now()
    const fresh = now - last > GAP
    last = now
    const dy = e.deltaMode ? e.deltaY * 40 : e.deltaY
    if (Math.abs(dy) < MIN_DELTA) return
    if (!fresh && now - fired < STEP) return
    fired = now
    const z = m.getZoom() + (dy < 0 ? 1 : -1)
    if (z < m.getMinZoom() || z > m.getMaxZoom()) return
    m.setZoomAround(m.mouseEventToContainerPoint(e), z)
  }
  box.addEventListener('wheel', onWheel, { passive: false })
  return () => box.removeEventListener('wheel', onWheel)
}
