// Bản đồ công trình (/map, admin + kế toán): mọi công trình CTV đã chụp (app + Messenger) trên nền ranh giới
// 34 tỉnh, thành từ 1/7/2025, màu theo trạng thái. Bảng bên: theo CTV (mỗi CTV đã chụp bao nhiêu công trình)
// hoặc theo tỉnh. Bấm 1 CTV / 1 tỉnh để lọc.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import mapApi from '@/api/mapApi'
import MapSearch from '@/components/MapSearch'
import SiteList from '@/components/SiteList'
import { fillCover, siteTip } from '@/utils/siteTip'
import { Button, Empty, Kpi, PageHead, PageState, Pill } from '@/components/ui'
import { AGE_FRESH, AGE_FRESH_LINE, AGE_STALE, esc, gmapUrl, hsId, isStale, vnDate } from '@/utils/format'

// Màu điểm theo trạng thái (giống tông nhãn trạng thái)
const TONE = { cho_duyet: 'wait', tra_lai: 'warn', da_duyet: 'ok', da_chi: 'done', tu_choi: 'bad' }
const VN_CENTER = [16.0, 106.2]
const LIVE_MS = 15000 // thời gian thực: tự tải lại mỗi 15 giây
const VN_BOUNDS = [
  [8.3, 102.1],
  [23.4, 109.6],
]


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
  const [tab, setTab] = useState('list') // list | ctv | tinh
  const [focus, setFocus] = useState(null)
  // Tô màu chấm: theo lần chụp gần nhất của công trình (mặc định) hoặc theo trạng thái hồ sơ
  const [mode, setMode] = useState('age')
  // Cập nhật theo thời gian thực: tự tải lại mỗi LIVE_MS, đánh dấu công trình mới xuất hiện
  const [live, setLive] = useState(false)
  const [liveAt, setLiveAt] = useState(null)
  const [fresh, setFresh] = useState(() => new Set())
  const known = useRef(null)
  useEffect(() => {
    if (!live) return
    const t = setInterval(() => !document.hidden && reload(), LIVE_MS)
    return () => clearInterval(t)
  }, [live, reload])
  useEffect(() => {
    if (!d) return
    const ids = new Set((d.points || []).map((p) => p.id))
    if (live && known.current) {
      const added = [...ids].filter((id) => !known.current.has(id))
      if (added.length) setFresh((s) => new Set([...s, ...added]))
      setLiveAt(new Date())
    }
    known.current = ids
  }, [d, live])
  const toggleLive = () => {
    if (live) {
      setLive(false)
      setFresh(new Set())
      return
    }
    setLive(true)
    setLiveAt(new Date())
    if (cur.to) set({ to: '' }) // xem tới hiện tại
  }
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
  const provOf = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
  const byProvince = [...provinces].sort((a, b) => b.total - a.total || a.name.localeCompare(b.name, 'vi'))

  return (
    <section className="stack">
      <PageHead
        title="Bản đồ công trình"
        sub={`Công trình CTV đã chụp (app và Messenger) trên 34 tỉnh, thành phố. Mỗi công trình chỉ nhận 1 hồ sơ trong ${d.dup_days} ngày.`}
      >
        <div className="map-tools">
          <button type="button" className={`btn btn-live ${live ? 'on' : ''}`} onClick={toggleLive} aria-pressed={live}>
            {live ? (
              <span className="live">
                <i /> Đang cập nhật theo thời gian thực
              </span>
            ) : (
              'Cập nhật theo thời gian thực'
            )}
          </button>
          {live && (
            <small className="muted">
              Tự tải lại mỗi {LIVE_MS / 1000} giây{liveAt ? ` · lần cuối ${liveAt.toLocaleTimeString('vi-VN')}` : ''}
              {fresh.size > 0 && <b className="text-bad"> · {fresh.size} công trình mới</b>}
            </small>
          )}
          <div className="history">
            <span>Lọc theo lịch sử</span>
            <input
              className="input"
              type="date"
              value={cur.from}
              max={cur.to || undefined}
              onChange={(e) => set({ from: e.target.value })}
              aria-label="Từ ngày"
            />
            <span className="muted">→</span>
            <input
              className="input"
              type="date"
              value={cur.to}
              min={cur.from || undefined}
              onChange={(e) => {
                if (e.target.value) setLive(false) // xem quá khứ: tắt thời gian thực
                set({ to: e.target.value })
              }}
              aria-label="Đến ngày"
            />
          </div>
        </div>
      </PageHead>
      {prov && prov.merged?.length > 0 && (
        <div className="note tone-wait">
          {prov.name} từ 1/7/2025 gồm: {prov.merged.join(', ')}. Công trình có địa chỉ ghi tên tỉnh cũ vẫn được tính vào {prov.name}.
        </div>
      )}

      <div className="map-grid">
        <div className="card map-card">
          <SiteMap
            points={points}
            provinces={provinces}
            selected={cur.province}
            onProvince={(code) => set({ province: code === cur.province ? '' : code })}
            fitKey={`${cur.ctv}|${cur.status}|${cur.from}|${cur.to}|${cur.province}`}
            color={mode === 'age' ? (p) => (isStale(p.last_at, d.dup_days) ? AGE_STALE : AGE_FRESH) : undefined}
            stroke={(p) => (fresh.has(p.id) ? '#dc2626' : mode === 'age' && !isStale(p.last_at, d.dup_days) ? AGE_FRESH_LINE : '#fff')}
            tip={(p) => siteTip(p, { staleDays: d.dup_days, prov: provOf[p.province], status: statuses[p.status]?.label })}
            staleDays={d.dup_days}
            paintKey={`${mode}|${fresh.size}`}
            focus={focus}
          />
          <div className="map-legend">
            <span className="seg seg-sm" role="tablist" aria-label="Tô màu">
              <button type="button" className={mode === 'age' ? 'on' : ''} onClick={() => setMode('age')}>
                Theo lần chụp
              </button>
              <button type="button" className={mode === 'status' ? 'on' : ''} onClick={() => setMode('status')}>
                Theo trạng thái
              </button>
            </span>
            {mode === 'age' ? (
              <>
                <span>
                  <i style={{ background: AGE_FRESH, boxShadow: `0 0 0 1px ${AGE_FRESH_LINE}` }} /> Chụp chưa quá {d.dup_days} ngày
                </span>
                <span>
                  <i style={{ background: AGE_STALE }} /> Quá {d.dup_days} ngày: cần cập nhật
                </span>
              </>
            ) : (
              Object.entries(TONE).map(([k, tone]) => (
                <span key={k}>
                  <i style={{ background: `var(--tone-${tone})` }} /> {statuses[k]?.label || k}
                </span>
              ))
            )}
          </div>
        </div>

        <div className="card map-aside">
          <div className="aside-filters">
            <select className="input" value={cur.province} onChange={(e) => set({ province: e.target.value })} aria-label="Tỉnh, thành">
              <option value="">Tỉnh thành: cả nước</option>
              {[...provinces]
                .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
                .map((p) => (
                  <option key={p.code} value={p.code}>
                    {p.name} ({p.total})
                  </option>
                ))}
            </select>
            <select className="input" value={cur.ctv} onChange={(e) => set({ ctv: e.target.value })} aria-label="Cộng tác viên">
              <option value="">Cộng tác viên: tất cả</option>
              {ctvs.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} ({c.total})
                </option>
              ))}
            </select>
            <select className="input" value={cur.status} onChange={(e) => set({ status: e.target.value })} aria-label="Trạng thái">
              <option value="">Trạng thái: tất cả</option>
              {Object.entries(statuses)
                .filter(([k]) => k !== 'nhap')
                .map(([k, s]) => (
                  <option key={k} value={k}>
                    {s.label}
                  </option>
                ))}
            </select>
            {(busy || Object.values(cur).some(Boolean)) && (
              <div className="aside-filters-foot">
                {busy ? <span className="muted">Đang tải…</span> : <span />}
                {Object.values(cur).some(Boolean) && (
                  <Button variant="ghost" type="button" onClick={() => setSp({})}>
                    Bỏ lọc
                  </Button>
                )}
              </div>
            )}
          </div>
          <div className="seg aside-tabs" role="tablist">
            <button type="button" role="tab" aria-selected={tab === 'list'} className={tab === 'list' ? 'on' : ''} onClick={() => setTab('list')}>
              Công trình <b className="mono">{points.length}</b>
            </button>
            <button type="button" role="tab" aria-selected={tab === 'ctv'} className={tab === 'ctv' ? 'on' : ''} onClick={() => setTab('ctv')}>
              Theo CTV <b className="mono">{ctvs.length}</b>
            </button>
            <button type="button" role="tab" aria-selected={tab === 'tinh'} className={tab === 'tinh' ? 'on' : ''} onClick={() => setTab('tinh')}>
              Theo tỉnh
            </button>
          </div>
          <div className="aside-body">
            {tab === 'list' && (
              <SiteList
                points={points}
                staleDays={d.dup_days}
                active={focus?.id}
                onPick={(p) => setFocus({ id: p.id, n: (focus?.n || 0) + 1 })}
                badge={(p) => (
                  <>
                    {p.vip && <span className="vip-tag">VIP</span>}
                    {fresh.has(p.id) && <span className="new-tag">MỚI</span>}
                  </>
                )}
                meta={(p) => (
                  <>
                    <b className="ctv-name">{p.ctv_name}</b> · {statuses[p.status]?.label || p.status}
                    {p.contractor ? ` · ${p.contractor}` : ''}
                  </>
                )}
              />
            )}
            {tab === 'ctv' &&
              (ctvs.length ? (
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
                          <strong className="ctv-name">{c.name}</strong> {c.inbox > 0 && <Pill tone="wait">Messenger {c.inbox}</Pill>}
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
              ) : (
                <Empty title="Chưa có công trình nào" />
              ))}
            {tab === 'tinh' && (
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
            )}
          </div>
        </div>
      </div>

      <div className="kpis">
        <Kpi label="Công trình trên bản đồ" value={points.length} sub={[sel?.name, prov?.name].filter(Boolean).join(' · ') || 'theo bộ lọc'} />
        <Kpi label="Cần cập nhật" value={points.filter((p) => isStale(p.last_at, d.dup_days)).length} sub={`quá ${d.dup_days} ngày`} />
        <Kpi label="Tỉnh, thành có công trình" value={`${withSites.length}/34`} />
        <Kpi label="CTV đã chụp" value={ctvs.filter((c) => c.total > 0).length} />
        <Kpi label={`Chụp trong ${d.dup_days} ngày`} value={sum('recent')} />
        <Kpi label="Bị chặn vì chụp trùng" value={sum('dups')} tone={sum('dups') ? 'bad' : undefined} />
      </div>
    </section>
  )
}

/** Bản đồ Leaflet: nền OpenStreetMap + ranh giới 34 tỉnh (tô theo số công trình) + mỗi công trình 1 chấm.
 *  pop(p): HTML popup của 1 điểm (link nội bộ: <a data-to="/đường/dẫn">); color(p) / stroke(p): màu chấm / viền;
 *  tip(p): HTML hiện khi rê chuột. Bỏ trống: kiểu trang admin (màu theo trạng thái).
 *  paintKey: đổi giá trị này để vẽ lại màu chấm (đổi cách tô, đánh dấu công trình mới).
 *  focus: { id, n } bay tới công trình id và mở popup (n tăng mỗi lần bấm, để bấm lại cùng dòng vẫn bay). */
export function SiteMap({ points, provinces, selected, onProvince, fitKey, pop, color, stroke, tip, staleDays = 30, paintKey = '', focus }) {
  const el = useRef(null)
  const map = useRef(null)
  const dots = useRef(null)
  const marks = useRef(new Map()) // id công trình -> chấm
  const getMap = useCallback(() => map.current, [])
  const fitted = useRef(null) // fitKey đã canh khung: tự tải lại (thời gian thực) không làm bản đồ nhảy
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
    // Nền OSM đi qua máy chủ mình (/tiles: lưu sẵn, không bị chặn như gọi thẳng OSM).
    // detectRetina: màn hình nét (laptop đời mới, điện thoại) lấy ô cấp chi tiết hơn để không bị mờ.
    L.tileLayer('/tiles/{z}/{x}/{y}.png', {
      maxZoom: 19,
      detectRetina: true,
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
      const a = e.target.closest('[data-to]')
      if (a) {
        e.preventDefault()
        navigate(a.dataset.to)
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
    marks.current = new Map()
    const latlngs = []
    for (const p of points) {
      const c = color ? color(p) : cssVar(`--tone-${TONE[p.status] || 'wait'}`, '#2563eb')
      const mk = L.circleMarker([p.lat, p.lng], { radius: 7, color: stroke ? stroke(p) : '#fff', weight: 1.5, fillColor: c, fillOpacity: 0.95 })
      if (tip) {
        // Thẻ cao ~310px: hiện phía trên chấm; không đủ chỗ thì phía dưới; vẫn không đủ thì sang trái / phải (không bị cắt). Đăng ký trước bindTooltip để chạy trước khi thẻ mở.
        mk.on('mouseover', () => {
          const tt = mk.getTooltip()
          if (!tt) return
          const { x, y } = m.latLngToContainerPoint(mk.getLatLng())
          const { x: W, y: H } = m.getSize()
          const [dir, off] = y >= 330 ? ['top', [0, -8]] : H - y >= 330 ? ['bottom', [0, 8]] : x < W / 2 ? ['right', [10, 0]] : ['left', [-10, 0]]
          tt.options.direction = dir
          tt.options.offset = off
        })
        mk.bindTooltip(tip(p), { direction: 'top', offset: [0, -8], className: 'site-tip-wrap', opacity: 1 })
        if (p.cover_id) mk.on('tooltipopen', (e) => fillCover(e.tooltip.getElement(), p.cover_id))
      }
      mk.bindPopup(
        pop ? pop(p) : `<div class="map-pop"><span class="mono">${hsId(p.id)}</span> · ${esc(labels[p.status]?.label || p.status)}` +
          `${p.source === 'inbox' ? ' · Messenger' : ''}<br><b>${esc(p.address || '(chưa có địa chỉ)')}</b>` +
          `${p.province ? `<br>${esc(provName(p.province))}` : ''}` +
          `${p.contractor ? `<br><span class="muted">Nhà thầu: ${esc(p.contractor)}</span>` : ''}` +
          `<br>CTV <b class="ctv-name">${esc(p.ctv_name)}</b>${p.vip ? ' <span class="vip-tag">VIP</span>' : ''} · <span class="mono">${esc(vnDate(p.at))}</span>` +
          `<br><a href="/houses/${p.id}" data-to="/houses/${p.id}">Mở hồ sơ</a>` +
          ` · <a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`,
      )
      mk.addTo(g)
      marks.current.set(p.id, mk)
      latlngs.push([p.lat, p.lng])
    }
    if (selected || fitted.current === fitKey) return // đã canh theo ranh giới tỉnh / đã canh với bộ lọc này
    fitted.current = fitKey
    // Chưa lọc: xem cả nước; lọc theo CTV / ngày / trạng thái: canh vừa các công trình
    const filtered = fitKey.replaceAll('|', '') !== ''
    if (filtered && latlngs.length) m.fitBounds(latlngs, { padding: [30, 30], maxZoom: 14 })
    else m.fitBounds(VN_BOUNDS)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitKey, points, labels, paintKey])

  // Bấm 1 dòng trong danh sách: bay tới chấm, mở popup
  useEffect(() => {
    const mk = focus && marks.current.get(focus.id)
    if (!mk) return
    map.current.setView(mk.getLatLng(), Math.max(map.current.getZoom(), 16))
    mk.openPopup()
  }, [focus])

  return (
    <div className="site-map-wrap">
      <MapSearch getMap={getMap} points={points} staleDays={staleDays} />
      <div ref={el} className="site-map" role="region" aria-label="Bản đồ công trình" />
    </div>
  )
}
