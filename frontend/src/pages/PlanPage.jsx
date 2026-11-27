// Xem quy hoạch (/quy-hoach, góp ý 20/11/2026): bản đồ quy hoạch sử dụng đất TP.HCM (lớp "bản đồ số" của
// thongtinquyhoach.hochiminhcity.gov.vn, máy chủ Sở QH-KT: sqhkt-qlqh.tphcm.gov.vn) phủ lên nền OSM của mình.
// Tìm địa chỉ -> phóng tới; chỉnh độ đậm lớp quy hoạch; bấm lô đất -> thẻ thông tin (thửa, chức năng, lộ giới, đồ án).
// Trang gốc không có link mở đúng vị trí, nên nút "Mở trang gốc" chỉ mở trang chủ.
// Lớp quy hoạch chỉ có ở TP.HCM (ranh cũ); ngoài vùng đó chỉ thấy nền.
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import L from 'leaflet'
import { stepWheelZoom } from '@/utils/wheelZoom'
import 'leaflet/dist/leaflet.css'
import mapApi from '@/api/mapApi'
import Icon from '@/components/ui/Icon'

const QH_SITE = 'https://thongtinquyhoach.hochiminhcity.gov.vn/'
const QH_TILES = 'https://sqhkt-qlqh.tphcm.gov.vn/api/tiles/bandoso/{z}/{x}/{y}'
const QH_BOUNDS = 'https://sqhkt-qlqh.tphcm.gov.vn/api/tiles/ranh_qhpk_tphcm_v2/{z}/{x}/{y}'
const HCM = [10.7769, 106.7009]
// Tra thông tin thửa đất tại 1 điểm (API trang gốc dùng; cho phép gọi từ map.vuathau.com.vn)
const QH_INFO = 'https://sqhkt-qlqh.tphcm.gov.vn/computing/930/api/v3.1/a-z/all'
const INFO_ZOOM = 16 // phóng gần từ cấp này mới tra (như trang gốc)

const parse = (v) => {
  if (typeof v !== 'string') return v
  try {
    return JSON.parse(v)
  } catch {
    return null
  }
}
const num = (v, d = 0) => (Number.isFinite(Number(v)) ? Number(v).toLocaleString('vi-VN', { maximumFractionDigits: d }) : '')
const geo = (v) => {
  const g = parse(v)
  if (Array.isArray(g)) {
    // chỉ có toạ độ (ranh thửa): [[[[lng, lat]...]]] = MultiPolygon, [[[lng, lat]...]] = Polygon
    const depth = (a) => (Array.isArray(a) ? 1 + depth(a[0]) : 0)
    const d = depth(g)
    return d === 4 ? { type: 'MultiPolygon', coordinates: g } : d === 3 ? { type: 'Polygon', coordinates: g } : null
  }
  return g && g.type ? g : null
}

// Thông tin 1 điểm: { lot: {so_to, so_thua, phuong, quan, dien_tich, ranh}, uses: [...], roads: [...], plans: [...], adjust: [...] } hoặc null (ngoài TP.HCM)
async function fetchInfo(lat, lng) {
  const r = await fetch(QH_INFO, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ Lat: String(lat), Lon: String(lng) }),
  })
  if (!r.ok) throw new Error(`Máy chủ quy hoạch lỗi ${r.status}`)
  const d = await r.json()
  const t = parse(d.ThongTinChung) || {}
  const feats = (k) => (parse(d[k]) || []).map((f) => ({ ...(f.properties || {}), geometry: f.geometry || null }))
  const uses = [...feats('QHPK'), ...feats('QHChiTiet')]
  if (!t.mathuadat && !uses.length) return null
  return {
    lot: { soTo: t.soto, soThua: t.sothua, phuong: t.tenphuongxa, quan: t.tenquanhuyen, dienTich: t.dientich, ranh: geo(t.ranh) },
    uses: uses.sort((a, b) => (b.tldientich || 0) - (a.tldientich || 0)),
    roads: feats('LoGioi'),
    plans: (t.dsttdoan || []).map((x) => ({ name: x.tendoan, soqd: x.soqd, ngay: x.ngayduyet, coquan: x.coquanpd })),
    adjust: feats('DCCB').map((x) => ({ name: x.tendccb, coquan: x.coquanpd })),
  }
}

function LotInfo({ d }) {
  const { lot, uses, roads, plans, adjust } = d
  return (
    <div className="qh-lot">
      {lot.soThua && (
        <div>
          <b>
            Thửa {lot.soThua}, tờ {lot.soTo}
          </b>
          <small className="d-block muted">
            {[lot.phuong, lot.quan].filter(Boolean).join(', ')}
            {lot.dienTich ? ` · ${num(lot.dienTich, 1)} m²` : ''}
          </small>
        </div>
      )}
      {uses.length > 0 && (
        <div>
          <span className="cap">Chức năng sử dụng đất</span>
          {uses.map((u, i) => (
            <div key={i} className="qh-use">
              <i style={{ background: u.rgbcolor ? `rgb(${u.rgbcolor})` : 'transparent' }} />
              <span>
                <b>{u.chucnang || u.maquyuoc}</b>
                <small className="d-block muted">
                  {[u.tldientich != null && `${num(u.tldientich, 1)}% thửa`, u.dientich && `${num(u.dientich, 1)} m²`, u.maopho && `ô phố ${u.maopho}`]
                    .filter(Boolean)
                    .join(' · ')}
                </small>
              </span>
            </div>
          ))}
        </div>
      )}
      {roads.length > 0 && (
        <div>
          <span className="cap">Lộ giới</span>
          {roads.map((r, i) => (
            <div key={i}>
              {r.tenduong}: <b>{r.logioi} m</b>
              {r.huongtiepgiap && <small className="muted"> · hướng {r.huongtiepgiap.toLowerCase()}</small>}
            </div>
          ))}
        </div>
      )}
      {plans.length > 0 && (
        <div>
          <span className="cap">Đồ án quy hoạch</span>
          {plans.map((p, i) => (
            <div key={i}>
              {p.name}
              <small className="d-block muted">{[p.soqd && `QĐ ${p.soqd}`, p.ngay, p.coquan].filter(Boolean).join(' · ')}</small>
            </div>
          ))}
        </div>
      )}
      {adjust.length > 0 && (
        <div>
          <span className="cap">Điều chỉnh cục bộ</span>
          {adjust.map((a, i) => (
            <div key={i}>
              {a.name}
              {a.coquan && <small className="d-block muted">{a.coquan}</small>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default function PlanPage() {
  const [sp, setSp] = useSearchParams()
  const el = useRef(null)
  const map = useRef(null)
  const plan = useRef(null)
  const zones = useRef(null)
  const mark = useRef(null)
  const [opacity, setOpacity] = useState(() => Number(sp.get('mo')) || 0.75)
  const [showZones, setShowZones] = useState(false)
  const [view, setView] = useState(null) // {lat, lng, z}
  const [q, setQ] = useState('')
  const [hits, setHits] = useState(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [info, setInfo] = useState(null) // {busy, err, data, far}
  const lotLayer = useRef(null)
  const seq = useRef(0)

  const lookup = async (m, ll) => {
    lotLayer.current?.remove()
    lotLayer.current = null
    if (m.getZoom() < INFO_ZOOM) {
      setInfo({ far: true })
      return
    }
    const n = ++seq.current
    setInfo({ busy: true })
    const dot = L.circleMarker(ll, { radius: 5, color: '#fff', weight: 2, fillColor: '#dc2626', fillOpacity: 1 })
    const g = L.layerGroup([dot]).addTo(m)
    lotLayer.current = g
    try {
      const data = await fetchInfo(ll.lat, ll.lng)
      if (n !== seq.current) return
      if (data?.lot.ranh) L.geoJSON(data.lot.ranh, { style: { color: '#dc2626', weight: 2.5, fillOpacity: 0.08 }, interactive: false }).addTo(g)
      setInfo({ data, none: !data })
    } catch (ex) {
      if (n === seq.current) setInfo({ err: ex.message === 'Failed to fetch' ? 'Không kết nối được máy chủ quy hoạch TP.HCM.' : ex.message })
    }
  }
  const closeInfo = () => {
    seq.current++
    lotLayer.current?.remove()
    lotLayer.current = null
    setInfo(null)
  }

  useEffect(() => {
    const lat = Number(sp.get('lat'))
    const lng = Number(sp.get('lng'))
    const z = Number(sp.get('z'))
    const m = L.map(el.current, { zoomControl: false, minZoom: 9, maxZoom: 20 }).setView(lat && lng ? [lat, lng] : HCM, z || 17)
    L.control.zoom({ position: 'topright' }).addTo(m)
    stepWheelZoom(m) // mỗi lần cuộn chuột chỉ 1 mức zoom
    L.tileLayer('/tiles/{z}/{x}/{y}.png', {
      maxZoom: 20,
      maxNativeZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
    }).addTo(m)
    plan.current = L.tileLayer(QH_TILES, {
      maxZoom: 20,
      opacity,
      attribution: `Quy hoạch: <a href="${QH_SITE}" target="_blank" rel="noreferrer">Sở Quy hoạch - Kiến trúc TP.HCM</a>`,
    }).addTo(m)
    zones.current = L.tileLayer(QH_BOUNDS, { maxZoom: 20, opacity: 0.5 })
    const sync = () => {
      const c = m.getCenter()
      setView({ lat: c.lat, lng: c.lng, z: m.getZoom() })
    }
    m.on('moveend', sync)
    m.on('click', (e) => lookup(m, e.latlng))
    sync()
    map.current = m
    return () => {
      m.remove()
      map.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    plan.current?.setOpacity(opacity)
  }, [opacity])
  useEffect(() => {
    const m = map.current
    if (!m || !zones.current) return
    if (showZones) zones.current.addTo(m)
    else zones.current.remove()
  }, [showZones])

  // Giữ vị trí trên địa chỉ trang (gửi link cho người khác xem đúng chỗ)
  useEffect(() => {
    if (!view) return
    const t = setTimeout(
      () => setSp({ lat: view.lat.toFixed(6), lng: view.lng.toFixed(6), z: String(view.z), ...(opacity !== 0.75 ? { mo: String(opacity) } : {}) }, { replace: true }),
      300,
    )
    return () => clearTimeout(t)
  }, [view, opacity, setSp])

  const go = (p) => {
    const m = map.current
    if (!m) return
    mark.current?.remove()
    mark.current = L.circleMarker([p.lat, p.lng], { radius: 7, color: '#fff', weight: 2, fillColor: '#2563eb', fillOpacity: 1 })
      .bindTooltip(p.name, { direction: 'top', offset: [0, -6] })
      .addTo(m)
    m.setView([p.lat, p.lng], Math.max(m.getZoom(), 17))
    setHits(null)
  }

  const search = async (e) => {
    e.preventDefault()
    const s = q.trim()
    if (s.length < 3) return
    setBusy(true)
    setErr('')
    try {
      const d = (await mapApi.searchPlace(s))?.data || {}
      const list = [
        ...(d.sites || []).map((x) => ({ name: x.address || x.contractor || `Công trình #${x.id}`, lat: x.lat, lng: x.lng })),
        ...(d.places || []),
      ]
      if (list.length === 1) go(list[0])
      else setHits(list)
    } catch (ex) {
      setErr(ex.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="qh">
      <div className="page-head">
        <h1>Xem quy hoạch</h1>
        <p>
          Bản đồ quy hoạch sử dụng đất TP.HCM (nguồn: Sở Quy hoạch - Kiến trúc TP.HCM). Phóng gần rồi bấm vào lô đất để xem số thửa, chức năng sử dụng đất, lộ giới và đồ án quy hoạch.
        </p>
      </div>
      <div className="qh-bar">
        <form className="qh-search" onSubmit={search}>
          <input className="input" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Tìm địa chỉ, số nhà, đường, phường…" aria-label="Tìm địa chỉ" />
          <button className="btn btn-primary" type="submit" disabled={busy || q.trim().length < 3}>
            {busy ? '…' : 'Tìm'}
          </button>
        </form>
        <label className="qh-opacity">
          <span>Độ đậm lớp quy hoạch</span>
          <input type="range" min="0" max="1" step="0.05" value={opacity} onChange={(e) => setOpacity(Number(e.target.value))} />
        </label>
        <label className="qh-check">
          <input type="checkbox" checked={showZones} onChange={(e) => setShowZones(e.target.checked)} /> Ranh quy hoạch phân khu
        </label>
        <a className="btn btn-ghost" href={QH_SITE} target="_blank" rel="noreferrer">
          <Icon name="external" size={16} /> Mở trang gốc
        </a>
      </div>
      <div className="qh-wrap">
        <div ref={el} className="qh-map" />
        {(hits || err) && (
          <div className="map-search-pop qh-pop">
            <div className="map-search-box">
              <button type="button" className="map-search-x" aria-label="Đóng" onClick={() => (setHits(null), setErr(''))}>
                ✕
              </button>
              {err ? (
                <span className="text-bad">{err}</span>
              ) : hits.length === 0 ? (
                <span className="muted">Không thấy. Thử ghi rõ hơn: số nhà, tên đường, phường, quận.</span>
              ) : (
                hits.map((p) => (
                  <button key={`${p.lat},${p.lng},${p.name}`} type="button" className="map-search-hit" onClick={() => go(p)}>
                    {p.name}
                  </button>
                ))
              )}
            </div>
          </div>
        )}
        {info && (
          <aside className="qh-info map-search-box" aria-live="polite">
            <button type="button" className="map-search-x" aria-label="Đóng" onClick={closeInfo}>
              ✕
            </button>
            {info.far && <span className="muted">Phóng gần thêm (tới mức thấy rõ từng lô) rồi bấm vào lô đất để xem thông tin quy hoạch.</span>}
            {info.busy && <span className="muted">Đang tra thông tin quy hoạch…</span>}
            {info.err && <span className="text-bad">{info.err}</span>}
            {info.none && <span className="muted">Chỗ này chưa có dữ liệu quy hoạch (ngoài địa bàn TP.HCM hoặc chưa số hoá).</span>}
            {info.data && <LotInfo d={info.data} />}
          </aside>
        )}
      </div>
      <p className="muted qh-note">Lớp quy hoạch chỉ có trong địa bàn TP.HCM (ranh trước sáp nhập). Bản đồ chỉ để tham khảo, không thay văn bản quy hoạch chính thức.</p>
    </div>
  )
}
