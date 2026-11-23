// Mọi hồ sơ (/houses): số liệu theo trạng thái, tìm theo mã / địa chỉ / nhà thầu / CTV, lọc ngày, phân trang.
// Bộ lọc nằm trên URL (?status=&q=&ctv=&from=&to=&page=), trùng tham số của GET /houses.
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import houseApi from '@/api/houseApi'
import { cleanParams } from '@/api/axiosClient'
import { Button, HouseTable, Kpi, PageHead, PageState, Pager } from '@/components/ui'
import { money } from '@/utils/format'

const FILTER_KEYS = ['status', 'q', 'ctv', 'province', 'from', 'to', 'page']

export default function OverviewPage() {
  const [sp, setSp] = useSearchParams()
  const params = Object.fromEntries(FILTER_KEYS.map((k) => [k, sp.get(k) || '']))
  const { data: d, err, busy, reload } = usePage(() => houseApi.list(params), [sp.toString()])
  const cfg = useCfg()
  const [q, setQ] = useState(sp.get('q') || '')

  if (!d) return <PageState err={err} retry={reload} />
  const f = { ...params, ...(d.filter || {}) }
  const go = (chg) => {
    const next = cleanParams({ ...f, page: '', ...chg })
    if (Number(next.page) <= 1) delete next.page
    if (!Number(next.ctv)) delete next.ctv // máy chủ có thể trả ctv: 0 khi không lọc
    setSp(Object.fromEntries(Object.entries(next).map(([k, v]) => [k, String(v)])))
  }
  const statuses = cfg?.status_order || Object.keys(cfg?.statuses || d.counts || {})
  const label = (s) => cfg?.statuses?.[s]?.label || s

  return (
    <section className="stack">
      <PageHead title="Công trình" sub={`${d.ctv_total ?? 0} cộng tác viên · ${d.ready_ctvs ?? 0} CTV đủ điều kiện thanh toán`} />

      <div className="kpis">
        {statuses.map((s) => (
          <Kpi key={s} label={label(s)} value={d.counts?.[s] ?? 0} sub={money(d.amounts?.[s])} tone={cfg?.statuses?.[s]?.tone} />
        ))}
      </div>

      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          go({ q, page: 1 })
        }}
      >
        <input className="input" placeholder="Tìm mã hồ sơ, địa chỉ, nhà thầu, tên / SĐT CTV" value={q} onChange={(e) => setQ(e.target.value)} />
        <select className="input" style={{ flex: '0 1 230px' }} value={f.province || ''} onChange={(e) => go({ province: e.target.value, page: 1 })} aria-label="Tỉnh, thành">
          <option value="">Cả nước (34 tỉnh, thành)</option>
          {[...(cfg?.provinces || [])]
            .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
            .map((p) => (
              <option key={p.code} value={p.code}>
                {p.name}
              </option>
            ))}
        </select>
        <label className="inline">
          Từ <input className="input" type="date" value={f.from || ''} onChange={(e) => go({ from: e.target.value, page: 1 })} />
        </label>
        <label className="inline">
          đến <input className="input" type="date" value={f.to || ''} onChange={(e) => go({ to: e.target.value, page: 1 })} />
        </label>
        <Button type="submit" loading={busy}>
          Tìm
        </Button>
        {(f.q || f.from || f.to || f.ctv || f.status || f.province) && (
          <Button
            variant="ghost"
            onClick={() => {
              setQ('')
              setSp({})
            }}
          >
            Xoá lọc
          </Button>
        )}
      </form>

      <div className="chips">
        <button type="button" className={`chip ${!f.status ? 'chip-on' : ''}`} onClick={() => go({ status: '', page: 1 })}>
          Tất cả
        </button>
        {statuses.map((s) => (
          <button key={s} type="button" className={`chip ${f.status === s ? 'chip-on' : ''}`} onClick={() => go({ status: s, page: 1 })}>
            {label(s)} <b>{d.chips?.[s] || 0}</b>
          </button>
        ))}
      </div>

      {d.ctv_name && <p className="muted">Đang xem hồ sơ của CTV: {d.ctv_name}</p>}
      <div className="card">
        <p className="muted">{d.found} hồ sơ</p>
        <HouseTable rows={d.rows || []} />
        <Pager page={d.page} pages={d.pages} onGo={(p) => go({ page: p })} />
      </div>
    </section>
  )
}
