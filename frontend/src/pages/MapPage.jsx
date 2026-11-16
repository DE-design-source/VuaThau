// Bản đồ công trình (/map, admin + kế toán): mọi công trình CTV đã chụp trên toàn Việt Nam (app + Messenger),
// màu theo trạng thái; bảng bên cạnh: mỗi CTV đã chụp bao nhiêu công trình. Bấm 1 CTV: chỉ hiện chỗ CTV đó đã check-in.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import mapApi from '@/api/mapApi'
import { Button, Empty, Kpi, PageHead, PageState, Pill } from '@/components/ui'
import { vnDate } from '@/utils/format'

// Màu điểm theo trạng thái (giống tông nhãn trạng thái)
const TONE = { cho_duyet: 'wait', tra_lai: 'warn', da_duyet: 'ok', da_chi: 'done', tu_choi: 'bad' }
const VN_CENTER = [16.0, 106.2]

const esc = (s = '') => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])

function toneColor(tone) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(`--tone-${tone}`).trim()
  return v || '#175cd3'
}

export default function MapPage() {
  const [sp, setSp] = useSearchParams()
  const cur = { ctv: sp.get('ctv') || '', status: sp.get('status') || '', from: sp.get('from') || '', to: sp.get('to') || '' }
  const { data: d, err, busy, reload } = usePage(() => mapApi.get(cur), [cur.ctv, cur.status, cur.from, cur.to])
  const cfg = useCfg()
  const [f, setF] = useState(cur)
  const set = (patch) => {
    const next = { ...cur, ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const ctvs = d.ctvs || []
  const sel = ctvs.find((c) => String(c.id) === cur.ctv)
  const statuses = cfg?.statuses || {}
  const sum = (k) => ctvs.reduce((n, c) => n + (c[k] || 0), 0)

  return (
    <section className="stack">
      <PageHead
        title="Bản đồ công trình"
        sub={`Mọi công trình CTV đã chụp (app và Messenger). Mỗi công trình chỉ nhận 1 hồ sơ trong ${d.dup_days} ngày.`}
      />
      <div className="kpis">
        <Kpi label="Công trình trên bản đồ" value={points.length} sub={sel ? `của ${sel.name}` : 'theo bộ lọc'} />
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
        <Button type="submit" loading={busy}>
          Lọc
        </Button>
        {(cur.ctv || cur.status || cur.from || cur.to) && (
          <Button
            variant="ghost"
            type="button"
            onClick={() => {
              setF({ ctv: '', status: '', from: '', to: '' })
              setSp({})
            }}
          >
            Bỏ lọc
          </Button>
        )}
      </form>

      <div className="map-layout">
        <div className="card map-card">
          <SiteMap points={points} fitKey={`${cur.ctv}|${cur.status}|${cur.from}|${cur.to}`} />
          <div className="map-legend">
            {Object.entries(TONE).map(([k, tone]) => (
              <span key={k}>
                <i style={{ background: `var(--tone-${tone})` }} /> {statuses[k]?.label || k}
              </span>
            ))}
          </div>
        </div>
        <div className="card map-side">
          <h2>Công trình mỗi CTV đã chụp</h2>
          {ctvs.length ? (
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
                        <small className="muted d-block">
                          {c.phone}
                          {c.last ? ` · gần nhất ${vnDate(c.last)}` : ''}
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
          )}
        </div>
      </div>
    </section>
  )
}

/** Bản đồ Leaflet (nền OpenStreetMap): mỗi công trình 1 chấm tròn, bấm xem nhanh rồi mở hồ sơ. */
function SiteMap({ points, fitKey }) {
  const el = useRef(null)
  const map = useRef(null)
  const layer = useRef(null)
  const navigate = useNavigate()
  const cfg = useCfg()

  useEffect(() => {
    const m = L.map(el.current, { preferCanvas: true, zoomControl: true }).setView(VN_CENTER, 6)
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
    }).addTo(m)
    layer.current = L.layerGroup().addTo(m)
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

  useEffect(() => {
    const m = map.current
    const g = layer.current
    if (!m || !g) return
    g.clearLayers()
    const latlngs = []
    for (const p of points) {
      const tone = TONE[p.status] || 'wait'
      const c = toneColor(tone)
      const mk = L.circleMarker([p.lat, p.lng], { radius: 7, color: '#fff', weight: 1.5, fillColor: c, fillOpacity: 0.9 })
      mk.bindPopup(
        `<div class="map-pop"><strong>#${p.id}</strong> ${esc(labels[p.status]?.label || p.status)}` +
          `${p.source === 'inbox' ? ' · Messenger' : ''}<br>${esc(p.address || '(chưa có địa chỉ)')}` +
          `${p.contractor ? `<br><span class="muted">Nhà thầu: ${esc(p.contractor)}</span>` : ''}` +
          `<br>CTV <strong>${esc(p.ctv_name)}</strong> · ${esc(vnDate(p.at))}` +
          `<br><a href="/houses/${p.id}" data-house="${p.id}">Mở hồ sơ</a></div>`,
      )
      mk.addTo(g)
      latlngs.push([p.lat, p.lng])
    }
    if (latlngs.length) m.fitBounds(latlngs, { padding: [30, 30], maxZoom: 15 })
    else m.setView(VN_CENTER, 6)
  }, [fitKey, points, labels])

  return <div ref={el} className="site-map" role="region" aria-label="Bản đồ công trình" />
}
