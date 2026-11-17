// Bản đồ công trình (/map, admin + kế toán), kiểu Airbnb: thanh tìm (Địa chỉ | Tỉnh thành | Cộng tác viên | Trạng thái),
// lưới thẻ công trình có ảnh bên trái, bản đồ nhãn giai đoạn thi công bên phải. Nút Bộ lọc: cập nhật theo thời gian thực,
// lọc theo lịch sử (từ ngày - đến ngày). Cuối danh sách: số liệu + thống kê theo CTV.
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import mapApi from '@/api/mapApi'
import Explorer from '@/components/explore/Explorer'
import { siteTip, stageLabel } from '@/utils/siteTip'
import { Button, Kpi, PageState, Pill } from '@/components/ui'
import { gmapUrl, hsId, isStale, vnDate } from '@/utils/format'

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

  const filterPanel = (
    <div className="ex-panel">
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
      fitKey={`${cur.ctv}|${cur.status}|${cur.from}|${cur.to}|${cur.province}`}
      selectedProvince={cur.province}
      onProvince={(code) => set({ province: code === cur.province ? '' : code })}
      filterPanel={filterPanel}
      filterCount={[cur.from, cur.to, live].filter(Boolean).length}
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
        {
          key: 'ctv',
          label: 'Cộng tác viên',
          value: cur.ctv,
          onChange: (v) => set({ ctv: v }),
          options: [{ value: '', label: 'Tất cả' }, ...ctvs.map((c) => ({ value: String(c.id), label: `${c.name} (${c.total})` }))],
        },
        {
          key: 'status',
          label: 'Trạng thái',
          value: cur.status,
          onChange: (v) => set({ status: v }),
          options: [
            { value: '', label: 'Tất cả' },
            ...Object.entries(statuses)
              .filter(([k]) => k !== 'nhap')
              .map(([k, s]) => ({ value: k, label: s.label })),
          ],
        },
      ]}
      pop={(p) =>
        siteTip(p, { staleDays: days, prov: name[p.province], status: statuses[p.status]?.label }) +
        `<div class="site-pop-actions"><a href="/houses/${p.id}" data-to="/houses/${p.id}">Mở hồ sơ</a>` +
        `<a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`
      }
      card={(p) => {
        const old = isStale(p.last_at || p.at, days)
        return {
          to: `/houses/${p.id}`,
          badge: fresh.has(p.id) ? 'Mới' : p.vip ? 'VIP · chụp đầu tiên' : old ? 'Cần cập nhật' : null,
          title: `${stageLabel(p.stage)} tại ${name[p.province] || 'chưa rõ tỉnh'}`,
          right: statuses[p.status]?.label || p.status,
          lines: [
            p.address || '(chưa có địa chỉ)',
            <>
              CTV <b className="ctv-name">{p.ctv_name}</b>
              {p.source === 'inbox' ? ' · Messenger' : ''}
            </>,
            p.contractor && `Nhà thầu: ${p.contractor}`,
          ],
          when: (
            <>
              <u>Chụp lần cuối {vnDate(p.last_at || p.at)}</u>
              {old && <b className="text-ok"> · cần cập nhật</b>}
            </>
          ),
          chips: [hsId(p.id)],
        }
      }}
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
