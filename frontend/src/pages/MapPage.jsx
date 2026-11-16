// Bản đồ công trình (/map, admin + kế toán): mọi công trình CTV đã chụp (app + Messenger) trên nền ranh giới
// 34 tỉnh, thành từ 1/7/2025, màu theo trạng thái. Bảng bên: theo CTV (mỗi CTV đã chụp bao nhiêu công trình)
// hoặc theo tỉnh. Bấm 1 CTV / 1 tỉnh để lọc.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import mapApi from '@/api/mapApi'
import { Button, Empty, Kpi, PageHead, PageState, Pill } from '@/components/ui'
import { hsId, vnDate } from '@/utils/format'

// Màu điểm theo trạng thái (giống tông nhãn trạng thái)
const TONE = { cho_duyet: 'wait', tra_lai: 'warn', da_duyet: 'ok', da_chi: 'done', tu_choi: 'bad' }
const VN_CENTER = [16.0, 106.2]
const VN_BOUNDS = [
  [8.3, 102.1],
  [23.4, 109.6],
]

const esc = (s = '') => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])

function cssVar(name, fallback) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

// Ranh giới 34 tỉnh: tải 1 lần, dùng lại khi đổi trang
let provincesGeo = null
function loadProvinces() {
  provincesGeo ??= fetch('/api/v1/geo/provinces').then((r) => (r.ok ? r.json() : null)).catch(() => null)
  return provincesGeo
}

export default function MapPage() {
  const [sp, setSp] = useSearchParams()
  const cur = {
    ctv: sp.get('ctv') || '',
    status: sp.get('status') || '',
    from: sp.get('from') || '',
    to: sp.get('to') || '',
    province: sp.get('province') || '',
  }
  const { data: d, err, busy, reload } = usePage(() => mapApi.get(cur), [cur.ctv, cur.status, cur.from, cur.to, cur.province])
  const cfg = useCfg()
  const [f, setF] = useState(cur)
  const [tab, setTab] = useState(cur.province ? 'tinh' : 'ctv')
  const set = (patch) => {
    const next = { ...cur, ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const ctvs = d.ctvs || []
  const provinces = d.provinces || []
  const sel = ctvs.find((c) => String(c.id) === cur.ctv)
  const prov = provinces.find((p) => p.code === cur.province)
  const statuses = cfg?.statuses || {}
  const sum = (k) => ctvs.reduce((n, c) => n + (c[k] || 0), 0)
  const withSites = provinces.filter((p) => p.total > 0)
  const byProvince = [...provinces].sort((a, b) => b.total - a.total || a.name.localeCompare(b.name, 'vi'))

  return (
    <section className="stack">
      <PageHead
        title="Bản đồ công trình"
        sub={`Công trình CTV đã chụp (app và Messenger) trên 34 tỉnh, thành phố. Mỗi công trình chỉ nhận 1 hồ sơ trong ${d.dup_days} ngày.`}
      />
      <div className="kpis">
        <Kpi label="Công trình trên bản đồ" value={points.length} sub={[sel?.name, prov?.name].filter(Boolean).join(' · ') || 'theo bộ lọc'} />
        <Kpi label="Tỉnh, thành có công trình" value={`${withSites.length}/34`} />
        <Kpi label="CTV đã chụp" value={ctvs.filter((c) => c.total > 0).length} />
        <Kpi label={`Chụp trong ${d.dup_days} ngày`} value={sum('recent')} />
        <Kpi label="Bị chặn vì chụp trùng" value={sum('dups')} tone={sum('dups') ? 'bad' : undefined} />
      </div>

      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          set({ from: f.from, to: f.to })
        }}
      >
        <select className="input" value={cur.province} onChange={(e) => set({ province: e.target.value })} aria-label="Tỉnh, thành">
          <option value="">Cả nước (34 tỉnh, thành)</option>
          {[...provinces]
            .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
            .map((p) => (
              <option key={p.code} value={p.code}>
                {p.name} ({p.total})
              </option>
            ))}
        </select>
        <select className="input" value={cur.ctv} onChange={(e) => set({ ctv: e.target.value })} aria-label="CTV">
          <option value="">Tất cả CTV</option>
          {ctvs.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name} ({c.total})
            </option>
          ))}
        </select>
        <select className="input" value={cur.status} onChange={(e) => set({ status: e.target.value })} aria-label="Trạng thái">
          <option value="">Mọi trạng thái</option>
          {Object.entries(statuses)
            .filter(([k]) => k !== 'nhap')
            .map(([k, s]) => (
              <option key={k} value={k}>
                {s.label}
              </option>
            ))}
        </select>
        <input className="input" type="date" value={f.from} onChange={(e) => setF({ ...f, from: e.target.value })} aria-label="Từ ngày" />
        <input className="input" type="date" value={f.to} onChange={(e) => setF({ ...f, to: e.target.value })} aria-label="Đến ngày" />
        <Button type="submit" variant="secondary" loading={busy}>
          Lọc ngày
        </Button>
        {Object.values(cur).some(Boolean) && (
          <Button
            variant="ghost"
            type="button"
            onClick={() => {
              setF({ ctv: '', status: '', from: '', to: '', province: '' })
              setSp({})
            }}
          >
            Bỏ lọc
          </Button>
        )}
      </form>

      {prov && prov.merged?.length > 0 && (
        <div className="note tone-wait">
          {prov.name} từ 1/7/2025 gồm: {prov.merged.join(', ')}. Công trình có địa chỉ ghi tên tỉnh cũ vẫn được tính vào {prov.name}.
        </div>
      )}

      <div className="map-layout">
        <div className="card map-card">
          <SiteMap
            points={points}
            provinces={provinces}
            selected={cur.province}
            onProvince={(code) => {
              setTab('tinh')
              set({ province: code === cur.province ? '' : code })
            }}
            fitKey={`${cur.ctv}|${cur.status}|${cur.from}|${cur.to}|${cur.province}`}
          />
          <div className="map-legend">
            {Object.entries(TONE).map(([k, tone]) => (
              <span key={k}>
                <i style={{ background: `var(--tone-${tone})` }} /> {statuses[k]?.label || k}
              </span>
            ))}
            <span className="spacer" />
            <span>Nền: ranh giới 34 tỉnh, thành (OCHA, 2025) · đậm = nhiều công trình</span>
          </div>
        </div>
        <div className="card map-side">
          <div className="seg" role="tablist">
            <button type="button" role="tab" aria-selected={tab === 'ctv'} className={tab === 'ctv' ? 'on' : ''} onClick={() => setTab('ctv')}>
              Theo CTV <b className="mono">{ctvs.length}</b>
            </button>
            <button type="button" role="tab" aria-selected={tab === 'tinh'} className={tab === 'tinh' ? 'on' : ''} onClick={() => setTab('tinh')}>
              Theo tỉnh <b className="mono">34</b>
            </button>
          </div>
          {tab === 'ctv' ? (
            ctvs.length ? (
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>CTV</th>
                      <th className="num">Công trình</th>
                      <th className="num">{d.dup_days} ngày</th>
                      <th className="num">Trùng</th>
                    </tr>
                  </thead>
                  <tbody>
                    {ctvs.map((c) => (
                      <tr
                        key={c.id}
                        className={String(c.id) === cur.ctv ? 'row-active' : ''}
                        onClick={() => set({ ctv: String(c.id) === cur.ctv ? '' : String(c.id) })}
                        style={{ cursor: 'pointer' }}
                      >
                        <td>
                          <strong>{c.name}</strong> {c.inbox > 0 && <Pill tone="wait">Messenger {c.inbox}</Pill>}
                          <small className="muted d-block mono">
                            {c.phone}
                            {c.last ? ` · ${vnDate(c.last)}` : ''}
                          </small>
                        </td>
                        <td className="num">
                          <strong>{c.total}</strong>
                        </td>
                        <td className="num">{c.recent}</td>
                        <td className="num">{c.dups ? <span className="text-bad">{c.dups}</span> : '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty title="Chưa có công trình nào" />
            )
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Tỉnh, thành</th>
                    <th className="num">Công trình</th>
                    <th className="num">{d.dup_days} ngày</th>
                  </tr>
                </thead>
                <tbody>
                  {byProvince.map((p) => (
                    <tr
                      key={p.code}
                      className={`${p.code === cur.province ? 'row-active' : ''} ${p.total ? '' : 'row-off'}`}
                      onClick={() => set({ province: p.code === cur.province ? '' : p.code })}
                      style={{ cursor: 'pointer' }}
                    >
                      <td>
                        <strong>{p.name}</strong>
                        {p.merged?.length > 0 && <small className="muted d-block">gồm {p.merged.join(', ')}</small>}
                      </td>
                      <td className="num">
                        <strong>{p.total}</strong>
                      </td>
                      <td className="num">{p.recent}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

/** Bản đồ Leaflet: nền OpenStreetMap + ranh giới 34 tỉnh (tô theo số công trình) + mỗi công trình 1 chấm. */
function SiteMap({ points, provinces, selected, onProvince, fitKey }) {
  const el = useRef(null)
  const map = useRef(null)
  const dots = useRef(null)
  const areas = useRef(null)
  const onProv = useRef(onProvince)
  const [geo, setGeo] = useState(null)
  const navigate = useNavigate()
  const cfg = useCfg()

  useEffect(() => {
    onProv.current = onProvince
  })

  useEffect(() => {
    let dead = false
    loadProvinces().then((g) => !dead && setGeo(g))
    return () => {
      dead = true
    }
  }, [])

  useEffect(() => {
    const m = L.map(el.current, { preferCanvas: true, zoomControl: true, minZoom: 5 }).setView(VN_CENTER, 6)
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> · Ranh giới: OCHA/HDX',
    }).addTo(m)
    const pane = m.createPane('areas')
    pane.style.zIndex = 350
    // Phóng gần (cấp phố): làm nhạt lớp tô tỉnh để đọc rõ đường phố
    const fade = () => {
      pane.style.opacity = m.getZoom() >= 10 ? '0.25' : '1'
    }
    m.on('zoomend', fade)
    areas.current = L.layerGroup().addTo(m)
    dots.current = L.layerGroup().addTo(m)
    map.current = m
    // Link trong popup: chuyển trang trong app (không tải lại cả trang)
    const onClick = (e) => {
      const a = e.target.closest('[data-house]')
      if (a) {
        e.preventDefault()
        navigate(`/houses/${a.dataset.house}`)
      }
    }
    const node = el.current
    node.addEventListener('click', onClick)
    return () => {
      node.removeEventListener('click', onClick)
      m.remove()
    }
  }, [navigate])

  const labels = useMemo(() => cfg?.statuses || {}, [cfg])
  const count = useMemo(() => Object.fromEntries(provinces.map((p) => [p.code, p])), [provinces])
  const provName = (code) => count[code]?.name || ''

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
        const p = count[ft.properties.code]
        const n = p?.total || 0
        const on = ft.properties.code === selected
        return {
          color: on ? amber : line,
          weight: on ? 2.5 : 0.8,
          opacity: on ? 1 : 0.45,
          fillColor: amber,
          fillOpacity: n ? 0.08 + 0.32 * (n / maxN) : 0.015,
        }
      },
      onEachFeature: (ft, lyr) => {
        const p = count[ft.properties.code]
        lyr.bindTooltip(
          `<b>${esc(ft.properties.name)}</b> · <span class="mono">${p?.total || 0}</span> công trình` +
            (p?.merged?.length ? `<br><small>gồm ${esc(p.merged.join(', '))}</small>` : ''),
          { sticky: true, direction: 'top', className: 'prov-tip' },
        )
        lyr.on('click', () => onProv.current?.(ft.properties.code))
      },
    })
    layer.addTo(g)
    if (selected) {
      const hit = layer.getLayers().find((l) => l.feature?.properties.code === selected)
      if (hit) map.current.fitBounds(hit.getBounds(), { padding: [20, 20] })
    }
  }, [geo, provinces, count, selected])

  // Chấm công trình
  useEffect(() => {
    const m = map.current
    const g = dots.current
    if (!m || !g) return
    g.clearLayers()
    const latlngs = []
    for (const p of points) {
      const tone = TONE[p.status] || 'wait'
      const c = cssVar(`--tone-${tone}`, '#2563eb')
      const mk = L.circleMarker([p.lat, p.lng], { radius: 6.5, color: '#fff', weight: 1.5, fillColor: c, fillOpacity: 0.95 })
      mk.bindPopup(
        `<div class="map-pop"><span class="mono">${hsId(p.id)}</span> · ${esc(labels[p.status]?.label || p.status)}` +
          `${p.source === 'inbox' ? ' · Messenger' : ''}<br><b>${esc(p.address || '(chưa có địa chỉ)')}</b>` +
          `${p.province ? `<br>${esc(provName(p.province))}` : ''}` +
          `${p.contractor ? `<br><span class="muted">Nhà thầu: ${esc(p.contractor)}</span>` : ''}` +
          `<br>CTV ${esc(p.ctv_name)} · <span class="mono">${esc(vnDate(p.at))}</span>` +
          `<br><a href="/houses/${p.id}" data-house="${p.id}">Mở hồ sơ</a></div>`,
      )
      mk.addTo(g)
      latlngs.push([p.lat, p.lng])
    }
    if (selected) return // đã canh theo ranh giới tỉnh
    // Chưa lọc: xem cả nước; lọc theo CTV / ngày / trạng thái: canh vừa các công trình
    const filtered = fitKey.replaceAll('|', '') !== ''
    if (filtered && latlngs.length) m.fitBounds(latlngs, { padding: [30, 30], maxZoom: 14 })
    else m.fitBounds(VN_BOUNDS)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitKey, points, labels])

  return <div ref={el} className="site-map" role="region" aria-label="Bản đồ công trình" />
}
