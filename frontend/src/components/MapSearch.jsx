// Ô tìm địa điểm trên bản đồ: gõ địa chỉ (vd "123 Lê Lợi, Huế") -> chọn kết quả -> bản đồ phóng tới đó,
// vẽ vòng bán kính và đếm bao nhiêu công trình trong vòng (bao nhiêu cái cần cập nhật).
import { useEffect, useRef, useState } from 'react'
import L from 'leaflet'
import mapApi from '@/api/mapApi'
import { isStale } from '@/utils/format'

const RADII = [500, 1000, 2000, 5000]
const km = (m) => (m >= 1000 ? `${m / 1000} km` : `${m} m`)

/** getMap(): bản đồ Leaflet; points: [{lat, lng, last_at?}]; staleDays: quá bấy nhiêu ngày là cần cập nhật. */
export default function MapSearch({ getMap, points, staleDays }) {
  const [q, setQ] = useState('')
  const [res, setRes] = useState(null) // null: chưa tìm; []: không thấy
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [place, setPlace] = useState(null) // {name, lat, lng}
  const [radius, setRadius] = useState(1000)
  const layer = useRef(null)

  // Vẽ vòng tìm + phóng tới
  useEffect(() => {
    const m = getMap()
    if (!m) return
    layer.current?.remove()
    layer.current = null
    if (!place) return
    const g = L.layerGroup().addTo(m)
    const c = L.circle([place.lat, place.lng], { radius, color: '#2563eb', weight: 2, fillColor: '#2563eb', fillOpacity: 0.06, interactive: false })
    c.addTo(g)
    L.circleMarker([place.lat, place.lng], { radius: 6, color: '#fff', weight: 2, fillColor: '#2563eb', fillOpacity: 1 }).addTo(g)
    layer.current = g
    m.fitBounds(c.getBounds(), { padding: [24, 24] })
  }, [place, radius, getMap])

  useEffect(() => () => layer.current?.remove(), [])

  const find = async (e) => {
    e.preventDefault()
    if (q.trim().length < 3) return
    setBusy(true)
    setErr('')
    try {
      const d = await mapApi.searchPlace(q.trim())
      const ps = d.places || []
      setRes(ps)
      if (ps.length === 1) setPlace(ps[0])
    } catch (ex) {
      setErr(ex.message)
    } finally {
      setBusy(false)
    }
  }

  let count = 0
  let stale = 0
  if (place) {
    const at = L.latLng(place.lat, place.lng)
    for (const p of points) {
      if (at.distanceTo([p.lat, p.lng]) <= radius) {
        count++
        if (p.last_at && isStale(p.last_at, staleDays)) stale++
      }
    }
  }

  return (
    <div className="map-search" onClick={(e) => e.stopPropagation()}>
      <form onSubmit={find} className="map-search-row">
        <input
          className="input"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Tìm kiếm địa chỉ, số nhà, đường, phường…"
          aria-label="Tìm địa điểm"
        />
        <button className="btn btn-primary" type="submit" disabled={busy || q.trim().length < 3}>
          {busy ? '…' : 'Tìm'}
        </button>
        {(place || res) && (
          <button
            className="btn btn-ghost"
            type="button"
            aria-label="Xoá tìm kiếm"
            onClick={() => {
              setPlace(null)
              setRes(null)
              setQ('')
            }}
          >
            ✕
          </button>
        )}
      </form>
      <div className="map-search-pop">
      {err && <div className="map-search-box text-bad">{err}</div>}
      {res && !place && (
        <div className="map-search-box">
          {res.length === 0 ? (
            <span className="muted">Không thấy địa điểm này. Thử ghi rõ hơn: số nhà, tên đường, phường, tỉnh.</span>
          ) : (
            res.map((p) => (
              <button key={`${p.lat},${p.lng}`} type="button" className="map-search-hit" onClick={() => setPlace(p)}>
                {p.name}
              </button>
            ))
          )}
        </div>
      )}
      {place && (
        <div className="map-search-box">
          <div className="map-search-name">{place.name}</div>
          <div className="map-search-count">
            Trong bán kính{' '}
            <select className="input input-sm" value={radius} onChange={(e) => setRadius(Number(e.target.value))} aria-label="Bán kính">
              {RADII.map((r) => (
                <option key={r} value={r}>
                  {km(r)}
                </option>
              ))}
            </select>
            : <b>{count}</b> công trình
            {stale > 0 && (
              <>
                {' '}
                · <b className="text-ok">{stale}</b> cần cập nhật
              </>
            )}
          </div>
          {res?.length > 1 && (
            <button type="button" className="map-search-hit muted" onClick={() => setPlace(null)}>
              Chọn kết quả khác ({res.length})
            </button>
          )}
        </div>
      )}
      </div>
    </div>
  )
}
