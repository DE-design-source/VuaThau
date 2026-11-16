// Báo cáo (/reports): tiền đã chi theo ngày / tuần / tháng, theo CTV, luồng hồ sơ, CTV mới; xuất Excel chi tiết.
// Bộ lọc trên URL (?from=&to=&group=day|week|month), trùng tham số của GET /reports.
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import reportApi from '@/api/reportApi'
import { toast } from '@/services/toast'
import { Button, Empty, Kpi, PageHead, PageState } from '@/components/ui'
import { money } from '@/utils/format'
import { saveBlob } from '@/utils/download'

const FLOW = [
  ['submitted', 'CTV gửi duyệt', 'wait'],
  ['approved', 'Admin duyệt', 'ok'],
  ['returned', 'Trả lại', 'warn'],
  ['rejected', 'Từ chối', 'bad'],
]

const GROUP = { day: 'Ngày', week: 'Tuần', month: 'Tháng' }

export default function ReportPage() {
  const [sp, setSp] = useSearchParams()
  const cur = { from: sp.get('from') || '', to: sp.get('to') || '', group: sp.get('group') || '' }
  const { data: d, err, busy, reload } = usePage(() => reportApi.get(cur), [cur.from, cur.to, cur.group])
  if (!d) return <PageState err={err} retry={reload} />
  const group = d.group || cur.group || 'day'
  const go = (chg) => {
    const next = { from: d.from || '', to: d.to || '', group, ...chg }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }
  const excel = () =>
    reportApi
      .exportFile({ from: d.from, to: d.to })
      .then((b) => saveBlob(b, `bao-cao-chi-${d.from}-den-${d.to}.xlsx`))
      .catch((e) => toast(e.message))

  return (
    <section className="stack">
      <PageHead title="Báo cáo" sub="Tiền đã chi cho cộng tác viên và luồng duyệt hồ sơ.">
        <Button variant="secondary" onClick={excel}>
          Tải Excel chi tiết
        </Button>
      </PageHead>

      <div className="card filters">
        <label className="inline">
          Từ <input className="input" type="date" value={d.from || ''} onChange={(e) => go({ from: e.target.value })} />
        </label>
        <label className="inline">
          đến <input className="input" type="date" value={d.to || ''} onChange={(e) => go({ to: e.target.value })} />
        </label>
        <div className="chips">
          {Object.entries(GROUP).map(([k, label]) => (
            <button key={k} type="button" className={`chip ${group === k ? 'chip-on' : ''}`} onClick={() => go({ group: k })}>
              Theo {label.toLowerCase()}
            </button>
          ))}
        </div>
        {busy && <span className="muted">Đang tải…</span>}
      </div>

      <div className="kpis">
        <Kpi label="Đã chi" value={money(d.total)} tone="done" />
        <Kpi label="Số lần chi" value={d.batches ?? 0} />
        <Kpi label="Hồ sơ được trả" value={d.houses ?? 0} />
        <Kpi label="CTV mới đăng ký" value={d.new_ctvs ?? 0} />
        {FLOW.map(([k, label, tone]) => (
          <Kpi key={k} label={label} value={d.flow?.[k] || 0} tone={tone} sub="hồ sơ" />
        ))}
      </div>

      <div className="grid-2">
        <div className="card stack">
          <h2>Theo {(GROUP[group] || 'ngày').toLowerCase()}</h2>
          {d.periods?.length ? (
            <table className="table">
              <thead>
                <tr>
                  <th>{GROUP[group] || 'Ngày'}</th>
                  <th className="num">Lần chi</th>
                  <th className="num">CTV</th>
                  <th className="num">Hồ sơ</th>
                  <th className="num">Tiền</th>
                </tr>
              </thead>
              <tbody>
                {d.periods.map((p) => (
                  <tr key={p.key}>
                    <td>{p.key}</td>
                    <td className="num">{p.batches}</td>
                    <td className="num">{p.ctvs}</td>
                    <td className="num">{p.houses}</td>
                    <td className="num">{money(p.total)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <Empty title="Chưa chi khoản nào trong khoảng này" />
          )}
        </div>
        <div className="card stack">
          <h2>Theo cộng tác viên</h2>
          {d.per_ctv?.length ? (
            <table className="table">
              <thead>
                <tr>
                  <th>CTV</th>
                  <th className="num">Lần</th>
                  <th className="num">Hồ sơ</th>
                  <th className="num">Tiền</th>
                </tr>
              </thead>
              <tbody>
                {d.per_ctv.map((c) => (
                  <tr key={c.ctv_id ?? c.name}>
                    <td>
                      {c.ctv_id ? <Link to={`/ctvs/${c.ctv_id}`}>{c.name}</Link> : c.name}
                      <small className="muted d-block">{c.username}</small>
                    </td>
                    <td className="num">{c.batches}</td>
                    <td className="num">{c.houses}</td>
                    <td className="num">{money(c.total)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <Empty title="Không có dữ liệu" />
          )}
        </div>
      </div>
    </section>
  )
}
