// Mọi hồ sơ (/tong-quan): số liệu theo trạng thái, tìm theo mã / địa chỉ / nhà thầu / CTV, lọc ngày, phân trang.
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import { Button, HouseTable, Kpi, PageHead, PageState, Pager } from '@/components/ui'
import { qs } from '@/api/ntClient'
import { money } from '@/utils/format'

export default function OverviewPage() {
  const [sp, setSp] = useSearchParams()
  const search = sp.toString()
  const { data: d, err, busy, reload } = usePage(`/tong-quan${search ? `?${search}` : ''}`)
  const cfg = useCfg()
  const [q, setQ] = useState(sp.get('q') || '')

  if (!d) return <PageState err={err} retry={reload} />
  const f = d.f || {}
  const go = (chg) => setSp(qs({ ...f, xem: '', ...chg }).slice(1))
  const statuses = Object.keys(cfg?.STATUS || d.cnt || {})

  return (
    <section className="stack">
      <PageHead title="Hồ sơ" sub={`${d.ctvs} cộng tác viên · ${d.ready} CTV đủ điều kiện thanh toán`} />

      <div className="kpis">
        {statuses.map((s) => (
          <Kpi key={s} label={cfg?.STATUS?.[s]?.[0] || s} value={d.cnt?.[s] ?? 0} sub={money(d.amt?.[s])} tone={cfg?.STATUS?.[s]?.[2]} />
        ))}
      </div>

      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          go({ q, trang: 1 })
        }}
      >
        <input className="input" placeholder="Tìm mã hồ sơ, địa chỉ, nhà thầu, tên / SĐT CTV" value={q} onChange={(e) => setQ(e.target.value)} />
        <label className="inline">
          Từ <input className="input" type="date" value={f.tu || ''} onChange={(e) => go({ tu: e.target.value, trang: 1 })} />
        </label>
        <label className="inline">
          đến <input className="input" type="date" value={f.den || ''} onChange={(e) => go({ den: e.target.value, trang: 1 })} />
        </label>
        <Button type="submit" loading={busy}>
          Tìm
        </Button>
        {(f.q || f.tu || f.den || f.ctv || f.trang_thai) && (
          <Button
            variant="ghost"
            onClick={() => {
              setQ('')
              setSp('')
            }}
          >
            Xoá lọc
          </Button>
        )}
      </form>

      <div className="chips">
        <button type="button" className={`chip ${!f.trang_thai ? 'chip-on' : ''}`} onClick={() => go({ trang_thai: '', trang: 1 })}>
          Tất cả
        </button>
        {statuses.map((s) => (
          <button key={s} type="button" className={`chip ${f.trang_thai === s ? 'chip-on' : ''}`} onClick={() => go({ trang_thai: s, trang: 1 })}>
            {cfg?.STATUS?.[s]?.[0] || s} <b>{d.chip?.[s] || 0}</b>
          </button>
        ))}
      </div>

      {d.ctv_name && <p className="muted">Đang xem hồ sơ của CTV: {d.ctv_name}</p>}
      <div className="card">
        <p className="muted">{d.found} hồ sơ</p>
        <HouseTable rows={d.rows || []} stats={d.pstats} />
        <Pager trang={d.trang} pages={d.pages} onGo={(t) => go({ trang: t })} />
      </div>
    </section>
  )
}
