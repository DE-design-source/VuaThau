// Tìm địa điểm trên bản đồ: địa chỉ (vd "123 Lê Lợi, Huế") -> chọn kết quả -> bản đồ phóng tới đó,
// vẽ vòng bán kính và đếm bao nhiêu công trình trong vòng (bao nhiêu cái cần cập nhật).
// Có `handle` (useRef): thanh tìm phía trên trang gọi handle.current.find(chữ); ở đây chỉ hiện kết quả nổi trên bản đồ.
// Không có `handle`: tự có ô nhập riêng.
import { useEffect, useImperativeHandle, useRef, useState } from 'react'
import L from 'leaflet'
import mapApi from '@/api/mapApi'
import { isStale } from '@/utils/format'

const RADII = [500, 1000, 2000, 5000]
const km = (m) => (m >= 1000 ? `${m / 1000} km` : `${m} m`)

/** getMap(): bản đồ Leaflet; points: [{lat, lng, last_at?}]; staleDays: quá bấy nhiêu ngày là cần cập nhật.
 *  onPlace({ name, lat, lng, radius } | null): vị trí đang chọn (trang lọc danh sách công trình quanh vị trí). */
export default function MapSearch({ getMap, points, staleDays, handle, onPlace }) {
  const [q, setQ] = useState('')
  const [res, setRes] = useState(null) // null: chưa tìm; { sites, places }
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

  // Báo trang: vị trí + bán kính đang chọn
  useEffect(() => {
    onPlace?.(place ? { ...place, radius } : null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [place, radius])

  const clear = () => {
    setPlace(null)
    setRes(null)
    setErr('')
    setQ('')
  }

  const run = async (text) => {
    const s = (text || '').trim()
    if (s.length < 3) return
    setBusy(true)
    setErr('')
    setPlace(null)
    try {
      const d = (await mapApi.searchPlace(s))?.data || {} // axiosClient trả phong bì { success, data }
      const sites = (d.sites || []).map((x) => ({ name: x.contractor || x.address || `Công trình #${x.id}`, sub: x.contractor ? x.address : '', lat: x.lat, lng: x.lng, site: true }))
      const places = d.places || []
      setRes({ sites, places })
      if (sites.length + places.length === 1) setPlace(sites[0] || places[0])
    } catch (ex) {
      setErr(ex.message)
    } finally {
      setBusy(false)
    }
  }

  // Thanh tìm phía trên trang gọi find(chữ) (chữ trống: xoá kết quả)
  useImperativeHandle(handle, () => ({ find: (s) => ((s || '').trim() ? run(s) : clear()), clear }))

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

  const close = (
    <button type="button" className="map-search-x" aria-label="Đóng" onClick={clear}>
      ✕
    </button>
  )

  return (
    <div className={`map-search ${handle ? 'headless' : ''}`} onClick={(e) => e.stopPropagation()}>
      {!handle && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            run(q)
          }}
          className="map-search-row"
        >
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
            <button className="btn btn-ghost" type="button" aria-label="Xoá tìm kiếm" onClick={clear}>
              ✕
            </button>
          )}
        </form>
      )}
      <div className="map-search-pop">
        {busy && handle && <div className="map-search-box muted">Đang tìm địa điểm…</div>}
        {err && (
          <div className="map-search-box text-bad">
            {close}
            {err}
          </div>
        )}
        {res && !place && (
          <div className="map-search-box">
            {close}
            {res.sites.length + res.places.length === 0 ? (
              <span className="muted">Không thấy. Thử ghi rõ hơn: số nhà, tên đường, phường, tỉnh; hoặc tên / SĐT nhà thầu.</span>
            ) : (
              <>
                {res.sites.length > 0 && <div className="map-search-group">Công trình trong hệ thống</div>}
                {res.sites.map((p, i) => (
                  <button key={`s${i}`} type="button" className="map-search-hit" onClick={() => setPlace(p)}>
                    <b>{p.name}</b>
                    {p.sub && <small className="d-block muted">{p.sub}</small>}
                  </button>
                ))}
                {res.places.length > 0 && <div className="map-search-group">Địa điểm</div>}
                {res.places.map((p) => (
                  <button key={`${p.lat},${p.lng}`} type="button" className="map-search-hit" onClick={() => setPlace(p)}>
                    {p.name}
                  </button>
                ))}
              </>
            )}
          </div>
        )}
        {place && (
          <div className="map-search-box">
            {close}
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
            {res && res.sites.length + res.places.length > 1 && (
              <button type="button" className="map-search-hit muted" onClick={() => setPlace(null)}>
                Chọn kết quả khác ({res.sites.length + res.places.length})
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
