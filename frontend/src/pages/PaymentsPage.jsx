// Phiếu chi đã lập (/batches): mỗi lần trả cho 1 CTV, kể cả phiếu đã huỷ.
import { Link } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import accountingApi from '@/api/accountingApi'
import { Empty, PageHead, PageState, Pill } from '@/components/ui'
import { money, vnTime } from '@/utils/format'

export default function PaymentsPage() {
  const { data: d, err, reload } = usePage(() => accountingApi.batches())
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []
  return (
    <section className="stack">
      <PageHead title="Phiếu chi" sub={`Tổng đã chi: ${money(d.total_paid)} · hiện 200 phiếu gần nhất`} />
      <div className="card">
        {rows.length ? (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Phiếu</th>
                  <th>CTV</th>
                  <th className="num">Hồ sơ</th>
                  <th className="num">Số tiền</th>
                  <th>Mã giao dịch</th>
                  <th>Ngày chi</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((b) => (
                  <tr key={b.id} className={b.status === 'huy' ? 'row-off' : ''}>
                    <td>
                      <Link to={`/batches/${b.id}`}>#{b.id}</Link> {b.status === 'huy' && <Pill tone="bad">Đã huỷ</Pill>}
                      {b.has_receipt ? <small className="muted d-block">có biên lai</small> : null}
                    </td>
                    <td>{b.ctv_id ? <Link to={`/ctvs/${b.ctv_id}`}>{b.ctv_name}</Link> : b.ctv_name || '—'}</td>
                    <td className="num">{b.house_count}</td>
                    <td className="num">
                      <strong>{money(b.total)}</strong>
                    </td>
                    <td>{b.ref || <span className="muted">—</span>}</td>
                    <td className="muted">{vnTime(b.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="Chưa có phiếu chi nào" />
        )}
      </div>
    </section>
  )
}
