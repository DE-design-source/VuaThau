// Bản đồ công trình (/map, admin + kế toán), kiểu Airbnb: thanh tìm (ô tìm | Tỉnh thành) + Bộ lọc (CTV, trạng thái, ngày...),
// tab Công trình | Cộng tác viên, bảng Lớp bản đồ (layout 21/11/2026).
// lưới thẻ công trình có ảnh bên trái, bản đồ nhãn mã hồ sơ bên phải. Nút Bộ lọc: cập nhật theo thời gian thực,
// lọc theo lịch sử (từ ngày - đến ngày). Cuối danh sách: số liệu + thống kê theo CTV.
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import mapApi from '@/api/mapApi'
import dezonApi from '@/api/dezonApi'
import Explorer from '@/components/explore/Explorer'
import { cardProps, siteTip } from '@/utils/siteTip'
import { Button, Kpi, PageState, Pill } from '@/components/ui'
import { gmapUrl, isStale, vnDate } from '@/utils/format'

const LIVE_MS = 15000 // thời gian thực: tự tải lại mỗi 15 giây

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
  const dz = usePage(() => dezonApi.projects({ province: cur.province }), [cur.province]) // lỗi: bỏ qua, chỉ thiếu lớp Dezon
  const cfg = useCfg()
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
  const set = (patch) => {
    const next = { ...cur, ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }
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
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const ctvs = d.ctvs || []
  const provinces = d.provinces || []
  const statuses = cfg?.statuses || {}
  const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
  const sum = (k) => ctvs.reduce((n, c) => n + (c[k] || 0), 0)
  const days = d.dup_days

  const sel = (label, value, onChange, options) => (
    <label className="field">
      <span>{label}</span>
      <select className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
  const filterPanel = (
    <div className="ex-panel">
      <div>
        <b>Lọc công trình</b>
        {sel('Cộng tác viên', cur.ctv, (v) => set({ ctv: v }), [{ value: '', label: 'Tất cả' }, ...ctvs.map((c) => ({ value: String(c.id), label: `${c.name} (${c.total})` }))])}
        {sel('Trạng thái hồ sơ', cur.status, (v) => set({ status: v }), [
          { value: '', label: 'Tất cả' },
          ...Object.entries(statuses)
            .filter(([k]) => k !== 'nhap')
            .map(([k, st]) => ({ value: k, label: st.label })),
        ])}
      </div>
      <div>
        <b>Cập nhật theo thời gian thực</b>
        <p className="muted">Tự tải lại mỗi {LIVE_MS / 1000} giây; công trình mới có nhãn đỏ.</p>
        <button type="button" className={`btn btn-live ${live ? 'on' : ''}`} onClick={toggleLive} aria-pressed={live}>
          {live ? (
            <span className="live">
              <i /> Đang cập nhật · bấm để tắt
            </span>
          ) : (
            'Bật cập nhật theo thời gian thực'
          )}
        </button>
        {live && (
          <small className="muted d-block">
            {liveAt ? `Lần cuối ${liveAt.toLocaleTimeString('vi-VN')}` : ''}
            {fresh.size > 0 && <b className="text-bad"> · {fresh.size} công trình mới</b>}
          </small>
        )}
      </div>
      <div>
        <b>Lọc theo lịch sử</b>
        <div className="ex-dates">
          <input className="input" type="date" value={cur.from} max={cur.to || undefined} onChange={(e) => set({ from: e.target.value })} aria-label="Từ ngày" />
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
      {Object.values(cur).some(Boolean) && (
        <Button variant="ghost" type="button" onClick={() => setSp({})}>
          Bỏ mọi bộ lọc
        </Button>
      )}
    </div>
  )

  return (
    <Explorer
      points={points}
      provinces={provinces}
      staleDays={days}
      busy={busy}
      fresh={fresh}
      dezon={cur.ctv || cur.status || cur.from || cur.to ? undefined : dz.data?.rows} // Dezon không thuộc bộ lọc CTV / trạng thái / ngày
      fitKey={`${cur.ctv}|${cur.status}|${cur.from}|${cur.to}|${cur.province}`}
      selectedProvince={cur.province}
      onProvince={(code) => set({ province: code === cur.province ? '' : code })}
      filterPanel={filterPanel}
      filterCount={[cur.from, cur.to, live, cur.ctv, cur.status].filter(Boolean).length}
      ctvPins={d.ctv_pins || []}
      customers={d.customers || []}
      selects={[
        {
          key: 'province',
          label: 'Tỉnh thành',
          value: cur.province,
          onChange: (v) => set({ province: v }),
          options: [
            { value: '', label: 'Cả nước' },
            ...[...provinces].sort((a, b) => a.name.localeCompare(b.name, 'vi')).map((p) => ({ value: p.code, label: `${p.name} (${p.total})` })),
          ],
        },
      ]}
      pop={(p) =>
        siteTip(p, { staleDays: days, prov: name[p.province], status: statuses[p.status]?.label, ctvLink: true }) +
        `<div class="site-pop-actions"><a href="/houses/${p.id}" data-to="/houses/${p.id}">Mở hồ sơ</a>` +
        `<a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`
      }
      card={(p) => cardProps(p, { to: `/houses/${p.id}`, prov: name[p.province], staleDays: days, fresh })}
      footer={
        <>
          <div className="kpis ex-kpis">
            <Kpi label="Công trình" value={points.length} sub="theo bộ lọc" />
            <Kpi label="Cần cập nhật" value={points.filter((p) => isStale(p.last_at || p.at, days)).length} sub={`quá ${days} ngày`} />
            <Kpi label="Tỉnh, thành có công trình" value={`${provinces.filter((p) => p.total > 0).length}/34`} />
            <Kpi label="CTV đã chụp" value={ctvs.filter((c) => c.total > 0).length} />
            <Kpi label={`Chụp trong ${days} ngày`} value={sum('recent')} />
            <Kpi label="Bị chặn vì chụp trùng" value={sum('dups')} tone={sum('dups') ? 'bad' : undefined} />
          </div>
          {ctvs.length > 0 && (
            <details className="card ex-stats">
              <summary>Thống kê theo cộng tác viên ({ctvs.length})</summary>
              <table className="table">
                <thead>
                  <tr>
                    <th>CTV</th>
                    <th className="num">Công trình</th>
                    <th className="num">{days} ngày</th>
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
            </details>
          )}
        </>
      }
    />
  )
}
