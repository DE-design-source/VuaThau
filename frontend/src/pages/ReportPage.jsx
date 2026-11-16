// Báo cáo (/bao-cao): tiền đã chi theo ngày / tuần / tháng, theo CTV, luồng hồ sơ, CTV mới; xuất Excel chi tiết.
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import ntClient, { qs } from '@/api/ntClient'
import { toast } from '@/services/toast'
import { Button, Empty, Kpi, PageHead, PageState } from '@/components/ui'
import { money } from '@/utils/format'

const FLOW = [
  ['Gửi duyệt', 'CTV gửi duyệt', 'wait'],
  ['Duyệt', 'Admin duyệt', 'ok'],
  ['Trả lại cho CTV sửa', 'Trả lại', 'warn'],
  ['Từ chối', 'Từ chối', 'bad'],
]

export default function ReportPage() {
  const [sp, setSp] = useSearchParams()
  const search = sp.toString()
  const { data: d, err, busy, reload } = usePage(`/bao-cao${search ? `?${search}` : ''}`)
  if (!d) return <PageState err={err} retry={reload} />
  const go = (chg) => setSp(qs({ tu: d.tu, den: d.den, nhom: d.nhom, ...chg }).slice(1))
  const excel = () =>
    ntClient.download(`/bao-cao/excel${qs({ tu: d.tu, den: d.den })}`, `bao-cao-chi-${d.tu}-den-${d.den}.xlsx`).catch((e) => toast(e.message))

  return (
    <section className="stack">
      <PageHead title="Báo cáo" sub="Tiền đã chi cho cộng tác viên và luồng duyệt hồ sơ.">
        <Button variant="secondary" onClick={excel}>
          Tải Excel chi tiết
        </Button>
      </PageHead>

      <div className="card filters">
        <label className="inline">
          Từ <input className="input" type="date" value={d.tu} onChange={(e) => go({ tu: e.target.value })} />
        </label>
        <label className="inline">
          đến <input className="input" type="date" value={d.den} onChange={(e) => go({ den: e.target.value })} />
        </label>
        <div className="chips">
          {Object.entries(d.GROUP || {}).map(([k, [label]]) => (
            <button key={k} type="button" className={`chip ${d.nhom === k ? 'chip-on' : ''}`} onClick={() => go({ nhom: k })}>
              Theo {label.toLowerCase()}
            </button>
          ))}
        </div>
        {busy && <span className="muted">Đang tải…</span>}
      </div>

      <div className="kpis">
        <Kpi label="Đã chi" value={money(d.total)} tone="done" />
        <Kpi label="Số lần chi" value={d.lan} />
        <Kpi label="Hồ sơ được trả" value={d.hs} />
        <Kpi label="CTV mới đăng ký" value={d.new_ctv} />
        {FLOW.map(([k, label, tone]) => (
          <Kpi key={k} label={label} value={d.flow?.[k] || 0} tone={tone} sub="hồ sơ" />
        ))}
      </div>

      <div className="grid-2">
        <div className="card stack">
          <h2>Theo {(d.GROUP?.[d.nhom]?.[0] || 'ngày').toLowerCase()}</h2>
          {d.periods?.length ? (
            <table className="table">
              <thead>
                <tr>
                  <th>{d.GROUP?.[d.nhom]?.[0]}</th>
                  <th className="num">Lần chi</th>
                  <th className="num">CTV</th>
                  <th className="num">Hồ sơ</th>
                  <th className="num">Tiền</th>
                </tr>
              </thead>
              <tbody>
                {d.periods.map((p) => (
                  <tr key={p.k}>
                    <td>{p.k}</td>
                    <td className="num">{p.lan}</td>
                    <td className="num">{p.ctv}</td>
                    <td className="num">{p.hs}</td>
                    <td className="num">{money(p.tien)}</td>
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
                  <tr key={c.id ?? c.name}>
                    <td>
                      {c.name}
                      <small className="muted d-block">{c.phone}</small>
                    </td>
                    <td className="num">{c.lan}</td>
                    <td className="num">{c.hs}</td>
                    <td className="num">{money(c.tien)}</td>
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
