// Bản đồ công trình (góp ý 19/11): mọi công trình là ghim giọt nước, màu theo việc cần làm:
// vàng: CTV đã chụp, chưa tới lúc chụp lại; xanh lá: cần chụp (quá hạn); xanh dương: công trình Dezon chưa CTV nào chụp;
// đỏ: hồ sơ mới xuất hiện (thời gian thực); navy: đang chọn / rê chuột. Mã hồ sơ / nhà thầu hiện dưới ghim khi phóng gần
// hoặc rê chuột. Ghim hồ sơ có khoá = id, ghim Dezon khoá = "dz" + id (trùng khoá thẻ trong danh sách).
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import L from 'leaflet'
import { stepWheelZoom } from '@/utils/wheelZoom'
import 'leaflet/dist/leaflet.css'
import 'maplibre-gl/dist/maplibre-gl.css'
import { maplibreGL } from '@maplibre/maplibre-gl-leaflet'
import MapSearch from '@/components/MapSearch'
import { dezonTip, fillCover, stageLabel } from '@/utils/siteTip'
import { esc, hsId, isStale } from '@/utils/format'
import { ctvBadge, cusBadge } from './pins'

const VN_CENTER = [16.0, 106.2]
const VN_BOUNDS = [
  [8.3, 102.1],
  [23.4, 109.6],
]
const FAR = 12 // phóng xa hơn cấp này: ghim nhỏ lại
const NEAR = 15 // phóng gần từ cấp này: hiện giai đoạn dưới ghim

// Ghim định vị (vẽ bằng SVG, màu theo class qua currentColor)
const PIN =
  '<svg viewBox="0 0 32 42" width="30" height="40" aria-hidden="true">' +
  '<path d="M16 1.5C8.3 1.5 2 7.7 2 15.4 2 25.8 16 40.5 16 40.5S30 25.8 30 15.4C30 7.7 23.7 1.5 16 1.5z" fill="currentColor" stroke="rgba(0,0,0,.28)" stroke-width="1.4"/>' +
  '<circle cx="16" cy="15.4" r="6.2" fill="#fff"/></svg>'

const HOVER_MS = 150 // rê chuột ngang qua ghim: chờ chút mới mở thẻ (đỡ nháy khi lướt qua nhiều ghim)
const LEAVE_MS = 450 // rời ghim / thẻ: chờ chừng này mới đóng, đủ thời gian rê chuột từ ghim lên thẻ

/**
 * Gắn thẻ cho 1 ghim (góp ý 24/11/2026: rê từ ghim sang thẻ thì thẻ tắt, không bấm được gì trong thẻ):
 *   rê chuột vào ghim: mở thẻ; rê tiếp lên thẻ: thẻ vẫn mở; rời cả ghim lẫn thẻ: đóng.
 *   bấm ghim: giữ thẻ mở, chỉ đóng khi bấm X / bấm ra ngoài bản đồ / bấm ghim khác.
 * st: { pinned } dùng chung cả bản đồ: đang giữ 1 thẻ thì rê chuột qua ghim khác không đè thẻ đó.
 * Thêm mk.showPop(giữ): mở thẻ từ ngoài (bấm ở danh sách, vẽ lại lớp ghim).
 */
function hoverPop(mk, html, opts, st) {
  mk.bindPopup(html, opts)
  mk.off('click', mk._openPopup, mk) // bỏ kiểu mặc định của Leaflet (bấm lần 2 thì tắt)
  const pop = mk.getPopup()
  let t = null
  const stop = () => clearTimeout(t)
  const show = (keep) => {
    stop()
    pop.options.autoPan = keep // mở do rê chuột: không kéo bản đồ (ghim chạy khỏi chuột)
    if (keep) st.pinned = mk
    if (!mk.isPopupOpen()) mk.openPopup()
  }
  const leave = () => {
    stop()
    t = setTimeout(() => st.pinned !== mk && mk.closePopup(), LEAVE_MS)
  }
  mk.on('mouseover', () => {
    stop()
    if (mk.isPopupOpen() || (st.pinned && st.pinned !== mk && st.pinned.isPopupOpen())) return
    t = setTimeout(() => show(false), HOVER_MS)
  })
  mk.on('mouseout', leave)
  mk.on('click', (e) => {
    L.DomEvent.stop(e)
    show(true)
  })
  mk.on('popupopen', () => {
    const el = pop.getElement()
    if (el && !el.dataset.hover) {
      el.dataset.hover = '1'
      el.addEventListener('mouseenter', stop)
      el.addEventListener('mouseleave', leave)
    }
  })
  mk.on('popupclose', () => {
    stop()
    if (st.pinned === mk) st.pinned = null
  })
  mk.on('remove', stop)
  mk.showPop = show
  return mk
}

// Vẽ lại 1 lớp ghim (tự tải lại mỗi 15 giây, đổi lớp...): nhớ thẻ đang mở của lớp đó để mở lại, không tắt ngang khi đang xem
function openOf(marks, own, st) {
  for (const [k, mk] of marks) if (own(String(k)) && mk.isPopupOpen()) return { k, keep: st.pinned === mk }
  return null
}
function reopen(marks, o) {
  const mk = o && marks.get(o.k)
  if (mk?.showPop) mk.showPop(o.keep)
}
const isSite = (k) => !/^(dz|ctv|kh)/.test(k)

function cssVar(name, fallback) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

// Nền bản đồ: Vietmap (Hoàng Sa, Trường Sa đúng chủ quyền; máy chủ có VIETMAP_API_KEY) nếu có, không thì nền cũ OpenFreeMap.
let basemap = null
function pickBasemap() {
  basemap ??= fetch('/map-style/vn.json').then(
    (r) => (r.ok ? { style: '/map-style/vn.json', attribution: '&copy; <a href="https://vietmap.vn" target="_blank" rel="noreferrer">VIETMAP</a>' } : null),
    () => null,
  )
  return basemap.then(
    (v) => v || { style: '/map-style.json', attribution: '&copy; OpenFreeMap &copy; OpenMapTiles &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>' },
  )
}

// Ranh giới 34 tỉnh: tải 1 lần, dùng lại khi đổi trang
let provincesGeo = null
function loadProvinces() {
  provincesGeo ??= fetch('/api/v1/geo/provinces').then((r) => (r.ok ? r.json() : null)).catch(() => null)
  return provincesGeo
}

/**
 * points: [{id, lat, lng, stage, last_at, cover_id, ...}]
 * pop(p): HTML thẻ khi bấm nhãn (có <img data-cover> thì tự tải ảnh). Link nội bộ: <a data-to="/đường/dẫn">.
 * active: id công trình đang rê chuột ở danh sách (nhãn đậm lên); onHover(id|null): rê chuột lên nhãn.
 * focus: { id, n } bay tới công trình và mở thẻ. fresh: Set id mới xuất hiện (thời gian thực).
 * search: tay cầm (useRef) để thanh tìm phía trên trang gọi search.current.find(chữ).
 */
export default function SiteMap({ points, provinces = [], selected, onProvince, fitKey, pop, staleDays = 30, active, onHover, focus, fresh, search, onPlace, dezon = [], ctvPins, ctvPop, customers, cusPop, overlay, ageColors = false }) {
  const cusLayer = useRef(null) // lớp khách hàng (ghim tròn đỏ chữ KH)
  const ctvLayer = useRef(null) // lớp cộng tác viên (ghim tròn chữ CTV / SALE, nằm trên ghim công trình)
  const el = useRef(null)
  const map = useRef(null)
  const pins = useRef(null)
  const dzPins = useRef(null) // lớp công trình Dezon (nằm dưới ghim hồ sơ)
  const marks = useRef(new Map()) // khoá ghim (id công trình, dz.., ctv.., kh..) -> marker
  const st = useRef({ pinned: null }) // thẻ đang được giữ mở (bấm ghim), xem hoverPop
  const areas = useRef(null)
  const fitted = useRef(null)
  const onProv = useRef(onProvince)
  const hover = useRef(onHover)
  const [geo, setGeo] = useState(null)
  const navigate = useNavigate()
  const getMap = useCallback(() => map.current, [])

  useEffect(() => {
    onProv.current = onProvince
    hover.current = onHover
  })

  useEffect(() => {
    let dead = false
    loadProvinces().then((g) => !dead && setGeo(g))
    return () => {
      dead = true
    }
  }, [])

  useEffect(() => {
    const m = L.map(el.current, { zoomControl: false, minZoom: 5 }).setView(VN_CENTER, 6)
    L.control.zoom({ position: 'topright' }).addTo(m)
    stepWheelZoom(m) // mỗi lần cuộn chuột chỉ 1 mức zoom
    // Nền vector: Vietmap nếu máy chủ có key (pickBasemap), không thì OpenFreeMap tô lại màu (/map-style.json, cùng file với app CTV).
    // Máy không vẽ được (không có WebGL) hoặc tải kiểu lỗi: quay về nền OSM qua máy chủ mình (/tiles).
    const osm = () =>
      L.tileLayer('/tiles/{z}/{x}/{y}.png', {
        maxZoom: 19,
        detectRetina: true,
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> · Ranh giới: OCHA/HDX',
      }).addTo(m)
    let vec = null
    let gone = false
    const fallback = () => {
      if (!vec) return
      m.removeLayer(vec)
      vec = null
      osm()
    }
    // Nền vector giữ chữ nét, cỡ chữ đọc được ở mọi mức phóng; nền ảnh OSM thì chữ nhỏ cố định, phóng gần là khó đọc
    // (góp ý 24/11/2026). Vietmap lỗi (ô bản đồ bị chặn / hết hạn mức) -> sang nền vector OpenFreeMap trước, cùng lỗi mới về OSM.
    const OFM = { style: '/map-style.json', attribution: '&copy; OpenFreeMap &copy; OpenMapTiles &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>' }
    const addVec = (b) => {
      try {
        const layer = maplibreGL({ style: b.style, attribution: `${b.attribution} · Ranh giới: OCHA/HDX` }).addTo(m)
        vec = layer
        const gl = layer.getMaplibreMap()
        let ok = false
        // có ô bản đồ vẽ được là nền chạy tốt
        gl.on('data', (e) => e.dataType === 'source' && e.tile && (ok = true))
        gl.on('error', (e) => {
          if (ok || gone || vec !== layer) return
          if (!/style|Failed to fetch|404|403|423|429|NetworkError|AJAXError/i.test(String(e?.error?.message || e?.error))) return
          if (b.style !== OFM.style) {
            m.removeLayer(vec)
            vec = null
            addVec(OFM)
          } else fallback()
        })
      } catch {
        fallback()
        if (!vec) osm()
      }
    }
    pickBasemap().then((b) => !gone && addVec(b))
    const pane = m.createPane('areas')
    pane.style.zIndex = 350
    const zoom = () => {
      pane.style.opacity = m.getZoom() >= 10 ? '0.06' : m.getZoom() >= 8 ? '0.4' : '1' // phóng gần: gần như ẩn lớp tô tỉnh, giữ màu nền
      el.current?.classList.toggle('zoom-far', m.getZoom() < FAR)
      el.current?.classList.toggle('zoom-near', m.getZoom() >= NEAR)
    }
    m.on('zoomend', zoom)
    zoom()
    areas.current = L.layerGroup().addTo(m)
    dzPins.current = L.layerGroup().addTo(m)
    pins.current = L.layerGroup().addTo(m)
    cusLayer.current = L.layerGroup().addTo(m)
    ctvLayer.current = L.layerGroup().addTo(m)
    map.current = m
    // Link trong thẻ: chuyển trang trong app (không tải lại cả trang)
    const onClick = (e) => {
      const a = e.target.closest('[data-to]')
      if (a) {
        e.preventDefault()
        navigate(a.dataset.to)
      }
    }
    const node = el.current
    node.addEventListener('click', onClick)
    return () => {
      gone = true
      node.removeEventListener('click', onClick)
      m.remove()
    }
  }, [navigate])

  const count = useMemo(() => Object.fromEntries(provinces.map((p) => [p.code, p])), [provinces])

  // Ranh giới tỉnh: tô theo số công trình, tỉnh đang chọn viền đậm
  useEffect(() => {
    const g = areas.current
    if (!g || !geo) return
    g.clearLayers()
    const maxN = Math.max(1, ...provinces.map((p) => p.total))
    const amber = cssVar('--amber', '#f5b301')
    const line = cssVar('--navy-3', '#1d3a66')
    const layer = L.geoJSON(geo, {
      pane: 'areas',
      style: (ft) => {
        const n = count[ft.properties.code]?.total || 0
        const on = ft.properties.code === selected
        return { color: on ? amber : line, weight: on ? 2.5 : 0.8, opacity: on ? 1 : 0.45, fillColor: amber, fillOpacity: n ? 0.06 + 0.24 * (n / maxN) : 0.01 }
      },
      onEachFeature: (ft, lyr) => {
        const p = count[ft.properties.code]
        lyr.bindTooltip(`<b>${esc(ft.properties.name)}</b> · <span class="mono">${p?.total || 0}</span> công trình`, {
          sticky: true,
          direction: 'top',
          className: 'prov-tip',
        })
        lyr.on('click', () => onProv.current?.(ft.properties.code))
      },
    })
    layer.addTo(g)
    if (selected) {
      const hit = layer.getLayers().find((l) => l.feature?.properties.code === selected)
      if (hit) map.current.fitBounds(hit.getBounds(), { padding: [20, 20] })
    }
  }, [geo, provinces, count, selected])

  // Nhãn công trình
  useEffect(() => {
    const m = map.current
    const g = pins.current
    if (!m || !g) return
    const was = openOf(marks.current, isSite, st.current)
    g.clearLayers()
    for (const k of [...marks.current.keys()]) if (isSite(String(k))) marks.current.delete(k) // giữ ghim CTV / khách / Dezon
    const latlngs = []
    for (const p of points) {
      // bản free (ageColors, góp ý 23/11/2026): vàng trong staleDays ngày, xanh lá quá staleDays, xanh dương quá 2 lần (2 tháng)
      const old = ageColors && isStale(p.last_at || p.at, staleDays * 2)
      const cls = ['site-pin', old ? 'old' : isStale(p.last_at || p.at, staleDays) ? 'stale' : '', fresh?.has(p.id) ? 'new' : ''].join(' ')
      const icon = L.divIcon({
        className: 'site-pin-icon',
        html: `<span class="${cls}" title="${esc(stageLabel(p.stage))}">${PIN}<span class="site-pin-label">${hsId(p.id)}</span></span>`,
        iconSize: null,
        popupAnchor: [0, -38],
      })
      const mk = L.marker([p.lat, p.lng], { icon, riseOnHover: true, keyboard: false })
      if (pop) {
        hoverPop(mk, pop(p), { className: 'site-pop', minWidth: 260, maxWidth: 260, autoPanPadding: [24, 24] }, st.current)
        if (p.cover_id) mk.on('popupopen', (e) => fillCover(e.popup.getElement(), p.cover_id))
      }
      mk.on('mouseover', () => hover.current?.(p.id))
      mk.on('mouseout', () => hover.current?.(null))
      mk.addTo(g)
      marks.current.set(p.id, mk)
      latlngs.push([p.lat, p.lng])
    }
    reopen(marks.current, was)
    if (selected || fitted.current === fitKey) return // đã canh theo tỉnh / đã canh với bộ lọc này (tự tải lại không làm nhảy)
    fitted.current = fitKey
    if (latlngs.length) m.fitBounds(latlngs, { padding: [40, 40], maxZoom: 14 })
    else m.fitBounds(VN_BOUNDS)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitKey, points, fresh?.size])

  // Công trình Dezon: ghim xanh dương, nằm dưới ghim hồ sơ; không tính vào canh khung bản đồ
  useEffect(() => {
    const g = dzPins.current
    if (!g) return
    const own = (k) => k.startsWith('dz')
    const was = openOf(marks.current, own, st.current)
    g.clearLayers()
    for (const k of [...marks.current.keys()]) if (own(String(k))) marks.current.delete(k)
    const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
    for (const p of dezon) {
      const icon = L.divIcon({
        className: 'site-pin-icon',
        html: `<span class="site-pin dz">${PIN}<span class="site-pin-label">${esc(p.contractor || 'Dezon')}</span></span>`,
        iconSize: null,
        popupAnchor: [0, -38],
      })
      const key = `dz${p.id}`
      const mk = L.marker([p.lat, p.lng], { icon, riseOnHover: true, keyboard: false, zIndexOffset: -500 })
      hoverPop(mk, dezonTip(p, { prov: name[p.province] }), { className: 'site-pop', minWidth: 260, maxWidth: 260, autoPanPadding: [24, 24] }, st.current)
      mk.addTo(g)
      mk.on('mouseover', () => hover.current?.(key))
      mk.on('mouseout', () => hover.current?.(null))
      marks.current.set(key, mk)
    }
    reopen(marks.current, was)
  }, [dezon, provinces])

  // Cộng tác viên (bộ icon 23/11/2026): ghim tròn chữ CTV cam (đặc: tích cực, viền: đã đăng ký), SALE tím (CTV kinh doanh);
  // tên hiện cạnh ghim. Nằm trên ghim công trình và khách hàng.
  useEffect(() => {
    const g = ctvLayer.current
    if (!g) return
    const own = (k) => k.startsWith('ctv')
    const was = openOf(marks.current, own, st.current)
    g.clearLayers()
    for (const k of [...marks.current.keys()]) if (own(String(k))) marks.current.delete(k)
    for (const c of ctvPins || []) {
      if (c.lat == null || c.lng == null) continue
      const cls = ['ctv-pin mk-pin', c.type === 'sales' ? 'sales' : '', c.status === 'active' ? 'solid' : '', c.approx ? 'approx' : ''].join(' ')
      const icon = L.divIcon({
        className: 'site-pin-icon',
        html: `<span class="${cls}" title="${esc(c.name)}">${ctvBadge(c)}<span class="site-pin-label">${esc(c.name)}</span></span>`,
        iconSize: null,
        popupAnchor: [0, -36],
      })
      const key = `ctv${c.id}`
      const mk = L.marker([c.lat, c.lng], { icon, riseOnHover: true, keyboard: false, zIndexOffset: c.status === 'active' ? 1100 : 1000 })
      if (ctvPop) hoverPop(mk, ctvPop(c), { className: 'site-pop', minWidth: 260, maxWidth: 280, autoPanPadding: [24, 24] }, st.current)
      mk.on('mouseover', () => hover.current?.(key))
      mk.on('mouseout', () => hover.current?.(null))
      mk.addTo(g)
      marks.current.set(key, mk)
    }
    reopen(marks.current, was)
  }, [ctvPins, ctvPop])

  // Khách hàng: ghim tròn chữ KH đỏ, tên cạnh ghim (dưới ghim CTV, trên ghim công trình)
  useEffect(() => {
    const g = cusLayer.current
    if (!g) return
    const own = (k) => k.startsWith('kh')
    const was = openOf(marks.current, own, st.current)
    g.clearLayers()
    for (const k of [...marks.current.keys()]) if (own(String(k))) marks.current.delete(k)
    for (const k of customers || []) {
      if (k.lat == null || k.lng == null) continue
      const icon = L.divIcon({
        className: 'site-pin-icon',
        html: `<span class="cus-pin mk-pin" title="${esc(k.name)}">${cusBadge()}<span class="site-pin-label">${esc(k.name)}</span></span>`,
        iconSize: null,
        popupAnchor: [0, -36],
      })
      const key = `kh${k.id}`
      const mk = L.marker([k.lat, k.lng], { icon, riseOnHover: true, keyboard: false, zIndexOffset: 500 })
      if (cusPop) hoverPop(mk, cusPop(k), { className: 'site-pop', minWidth: 240, maxWidth: 280, autoPanPadding: [24, 24] }, st.current)
      mk.on('mouseover', () => hover.current?.(key))
      mk.on('mouseout', () => hover.current?.(null))
      mk.addTo(g)
      marks.current.set(key, mk)
    }
    reopen(marks.current, was)
  }, [customers, cusPop])

  // Rê chuột ở danh sách: nhãn tương ứng đậm lên, nổi lên trên
  useEffect(() => {
    for (const [id, mk] of marks.current) {
      const on = id === active
      mk.getElement()?.querySelector('.site-pin, .ctv-pin, .cus-pin')?.classList.toggle('on', on)
      if (String(id).startsWith('ctv')) mk.setZIndexOffset(on ? 1500 : 1000)
      else if (String(id).startsWith('kh')) mk.setZIndexOffset(on ? 1500 : 500)
      else mk.setZIndexOffset(on ? 1000 : 0)
    }
  }, [active, points, ctvPins, customers])

  // Bấm "Xem trên bản đồ": bay tới, mở thẻ
  useEffect(() => {
    const mk = focus && marks.current.get(focus.id)
    if (!mk) return
    map.current.setView(mk.getLatLng(), Math.max(map.current.getZoom(), 16))
    if (mk.showPop) mk.showPop(true) // giữ thẻ mở tới khi bấm X / bấm ra ngoài
    else mk.openPopup()
  }, [focus])

  return (
    <div className="site-map-wrap">
      <div ref={el} className="site-map" role="region" aria-label="Bản đồ công trình" />
      {overlay}
      <MapSearch getMap={getMap} points={points} staleDays={staleDays} handle={search} onPlace={onPlace} />
    </div>
  )
}
