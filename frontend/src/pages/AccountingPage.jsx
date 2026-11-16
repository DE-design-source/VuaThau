// Kế toán (/accounting): CTV đủ điều kiện thanh toán (đủ số hồ sơ đã duyệt), xuất file chuyển khoản, xác nhận đã trả hàng loạt.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useCfg from '@/hooks/useCfg'
import accountingApi from '@/api/accountingApi'
import { toast } from '@/services/toast'
import { Button, Empty, Kpi, PageHead, PageState } from '@/components/ui'
import { bankParts, money, today } from '@/utils/format'
import { saveBlob } from '@/utils/download'

export default function AccountingPage() {
  const { data: d, err, reload } = usePage(() => accountingApi.overview())
  const [act, busy] = useAct(reload)
  const cfg = useCfg()
  const [sel, setSel] = useState(() => new Set())
  const [ref, setRef] = useState('')
  if (!d) return <PageState err={err} retry={reload} />
  const ready = d.ready || []
  const picked = ready.filter((r) => sel.has(r.ctv_id))
  const sum = picked.reduce((s, r) => s + r.total, 0)
  const toggle = (id) =>
    setSel((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  const all = ready.length > 0 && sel.size === ready.length
  const file = (format) =>
    accountingApi
      .exportFile({ format, ids: [...sel] })
      .then((b) => saveBlob(b, format === 'xlsx' ? `chuyen-khoan-${today()}.xlsx` : `can-thanh-toan-${today()}.csv`))
      .catch((e) => toast(e.message))

  const payMany = async () => {
    if (!window.confirm(`Xác nhận ĐÃ chuyển khoản cho ${picked.length} CTV, tổng ${money(sum)}? Mỗi CTV sẽ có 1 phiếu chi.`)) return
    // gửi kèm đúng tổng tiền đang thấy: dữ liệu vừa đổi thì máy chủ từ chối (409), tránh trả nhầm
    const items = picked.map((r) => ({ ctv_id: r.ctv_id, total: r.total }))
    const j = await act('many', () => accountingApi.payMany({ ref, items }), { go: '/batches' })
    if (j) setSel(new Set())
  }

  return (
    <section className="stack">
      <PageHead title="Thanh toán cho cộng tác viên" sub={`CTV có từ ${cfg?.pay_min ?? 5} hồ sơ đã duyệt thì được trả gộp mọi hồ sơ đang chờ, kèm thưởng/trừ.`} />
      <div className="kpis">
        <Kpi label="CTV đủ điều kiện" value={ready.length} tone="ok" />
        <Kpi label="Tổng cần trả" value={money(d.total)} tone="ok" />
        <Kpi label="CTV chưa đủ hồ sơ" value={(d.waiting || []).length} tone="wait" />
      </div>

      <div className="card stack">
        <div className="row-between">
          <h2>Đủ điều kiện thanh toán</h2>
          <div className="row-wrap">
            <Button variant="secondary" onClick={() => file('xlsx')} disabled={!ready.length}>
              Tải Excel chuyển khoản{sel.size ? ` (${sel.size})` : ''}
            </Button>
            <Button variant="ghost" onClick={() => file('csv')} disabled={!ready.length}>
              CSV
            </Button>
          </div>
        </div>
        {ready.length ? (
          <>
            <Table rows={ready} selectable sel={sel} toggle={toggle} all={all} setAll={() => setSel(all ? new Set() : new Set(ready.map((r) => r.ctv_id)))} />
            <div className="pay-bar">
              <input className="input grow" placeholder="Mã giao dịch / ghi chú chung (tuỳ chọn)" value={ref} onChange={(e) => setRef(e.target.value)} />
              <Button onClick={payMany} disabled={!picked.length} loading={busy === 'many'}>
                Đã chuyển khoản {picked.length || ''} CTV · {money(sum)}
              </Button>
            </div>
            <p className="muted">Mẹo: tải Excel, chuyển khoản hàng loạt trên ngân hàng, rồi chọn đúng các CTV đó và bấm xác nhận. Muốn trả từng người: bấm tên CTV.</p>
          </>
        ) : (
          <Empty title="Chưa có CTV nào đủ điều kiện" text="Khi admin duyệt đủ hồ sơ cho 1 CTV, người đó sẽ hiện ở đây." />
        )}
      </div>

      {(d.waiting || []).length > 0 && (
        <div className="card stack">
          <h2>Chưa đủ hồ sơ</h2>
          <Table rows={d.waiting} />
        </div>
      )}
    </section>
  )
}

function Table({ rows, selectable, sel, toggle, all, setAll }) {
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            {selectable && (
              <th>
                <input type="checkbox" checked={all} onChange={setAll} aria-label="Chọn tất cả" />
              </th>
            )}
            <th>CTV</th>
            <th>Tài khoản nhận</th>
            <th className="num">Hồ sơ</th>
            <th className="num">Số tiền</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const b = bankParts(r.bank)
            return (
              <tr key={r.ctv_id}>
                {selectable && (
                  <td>
                    <input type="checkbox" checked={sel.has(r.ctv_id)} onChange={() => toggle(r.ctv_id)} aria-label={`Chọn ${r.name}`} />
                  </td>
                )}
                <td>
                  <Link to={`/payouts/${r.ctv_id}`}>
                    <strong>{r.name}</strong>
                  </Link>
                  <small className="muted d-block">{r.username}</small>
                </td>
                <td>{b.owner ? `${b.name} · ${b.no} · ${b.owner}` : r.bank || <span className="text-bad">Chưa có tài khoản</span>}</td>
                <td className="num">{r.house_count}</td>
                <td className="num">
                  <strong>{money(r.total)}</strong>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
