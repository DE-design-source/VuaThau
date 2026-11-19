// Bản đồ công trình (thiết kế 19/11): mỗi hồ sơ là 1 nhãn viên thuốc ghi mã (HS-00121), đáy nhãn chỉ đúng vị trí:
// vàng: chụp chưa quá hạn; xanh lá: cần cập nhật; đỏ: mới xuất hiện (thời gian thực); đen: đang chọn / rê chuột.
// Phóng xa (< cấp 12): nhãn thu thành chấm cho đỡ chồng. Bấm nhãn: thẻ có hình đại diện.
// Công trình nhà thầu đăng trên dezon.vn: ghim giọt nước xanh dương (lớp riêng, prop dezon), giai đoạn hiện khi phóng gần.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import 'maplibre-gl/dist/maplibre-gl.css'
import { maplibreGL } from '@maplibre/maplibre-gl-leaflet'
import MapSearch from '@/components/MapSearch'
import { dezonTip, fillCover, stageLabel } from '@/utils/siteTip'
import { esc, hsId, isStale } from '@/utils/format'

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
export default function SiteMap({ points, provinces = [], selected, onProvince, fitKey, pop, staleDays = 30, active, onHover, focus, fresh, search, onPlace, dezon = [] }) {
  const el = useRef(null)
  const map = useRef(null)
  const pins = useRef(null)
  const dzPins = useRef(null) // lớp công trình Dezon (nằm dưới ghim hồ sơ)
  const marks = useRef(new Map()) // id công trình -> marker
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
    pickBasemap().then((b) => {
      if (gone) return
      try {
        vec = maplibreGL({ style: b.style, attribution: `${b.attribution} · Ranh giới: OCHA/HDX` }).addTo(m)
        const gl = vec.getMaplibreMap()
        let loaded = false
        gl.once('load', () => (loaded = true))
        gl.on('error', (e) => !loaded && /style|Failed to fetch|404/i.test(String(e?.error?.message || e?.error)) && fallback())
      } catch {
        fallback()
        if (!vec) osm()
      }
    })
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
    g.clearLayers()
    marks.current = new Map()
    const latlngs = []
    for (const p of points) {
      const cls = ['site-tag', isStale(p.last_at || p.at, staleDays) ? 'stale' : '', fresh?.has(p.id) ? 'new' : ''].join(' ')
      const icon = L.divIcon({
        className: 'site-pin-icon',
        html: `<span class="${cls}" title="${esc(stageLabel(p.stage))}">${hsId(p.id)}</span>`,
        iconSize: null,
        popupAnchor: [0, -30],
      })
      const mk = L.marker([p.lat, p.lng], { icon, riseOnHover: true, keyboard: false })
      if (pop) {
        mk.bindPopup(pop(p), { className: 'site-pop', minWidth: 260, maxWidth: 260, autoPanPadding: [24, 24] })
        if (p.cover_id) mk.on('popupopen', (e) => fillCover(e.popup.getElement(), p.cover_id))
      }
      mk.on('mouseover', () => hover.current?.(p.id))
      mk.on('mouseout', () => hover.current?.(null))
      mk.addTo(g)
      marks.current.set(p.id, mk)
      latlngs.push([p.lat, p.lng])
    }
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
    g.clearLayers()
    const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
    for (const p of dezon) {
      const icon = L.divIcon({
        className: 'site-pin-icon',
        html: `<span class="site-pin dz">${PIN}<span class="site-pin-label">${esc(p.contractor || 'Dezon')}</span></span>`,
        iconSize: null,
        popupAnchor: [0, -38],
      })
      L.marker([p.lat, p.lng], { icon, riseOnHover: true, keyboard: false, zIndexOffset: -500 })
        .bindPopup(dezonTip(p, { prov: name[p.province] }), { className: 'site-pop', minWidth: 260, maxWidth: 260, autoPanPadding: [24, 24] })
        .addTo(g)
    }
  }, [dezon, provinces])

  // Rê chuột ở danh sách: nhãn tương ứng đậm lên, nổi lên trên
  useEffect(() => {
    for (const [id, mk] of marks.current) {
      const on = id === active
      mk.getElement()?.querySelector('.site-tag')?.classList.toggle('on', on)
      mk.setZIndexOffset(on ? 1000 : 0)
    }
  }, [active, points])

  // Bấm "Xem trên bản đồ": bay tới, mở thẻ
  useEffect(() => {
    const mk = focus && marks.current.get(focus.id)
    if (!mk) return
    map.current.setView(mk.getLatLng(), Math.max(map.current.getZoom(), 16))
    mk.openPopup()
  }, [focus])

  return (
    <div className="site-map-wrap">
      <div ref={el} className="site-map" role="region" aria-label="Bản đồ công trình" />
      <MapSearch getMap={getMap} points={points} staleDays={staleDays} handle={search} onPlace={onPlace} />
    </div>
  )
}
