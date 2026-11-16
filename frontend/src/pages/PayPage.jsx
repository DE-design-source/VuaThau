// Thanh toán 1 CTV (/thanh-toan/:id): mọi hồ sơ đã duyệt + thưởng/trừ chưa trả, nội dung chuyển khoản, xác nhận đã trả.
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useCfg from '@/hooks/useCfg'
import { toast } from '@/services/toast'
import { Button, HouseTable, PageHead, PageState } from '@/components/ui'
import { money } from '@/utils/format'
import AdjustBox from './parts/AdjustBox'

export default function PayPage() {
  const { id } = useParams()
  const { data: d, err, reload } = usePage(`/thanh-toan/${id}`)
  const [act, busy] = useAct(reload)
  const cfg = useCfg()
  const [ref, setRef] = useState('')
  if (!d) return <PageState err={err} retry={reload} />
  const { ctv, bank, rows = [], adjs = [] } = d
  const enough = rows.length >= (cfg?.PAY_MIN ?? 5) && d.total > 0
  const copy = (t) => navigator.clipboard?.writeText(String(t)).then(() => toast(`Đã chép: ${t}`), () => {})

  const pay = () => {
    if (!window.confirm(`Xác nhận ĐÃ chuyển ${money(d.total)} cho ${ctv.name}?`)) return
    // gửi kèm đúng danh sách đang thấy: máy chủ từ chối nếu dữ liệu vừa đổi, tránh trả nhầm
    act('pay', `/thanh-toan/${id}`, { ids: rows.map((r) => r.id), adj: adjs.map((a) => a.id), ref })
  }

  return (
    <section className="stack">
      <PageHead title={`Thanh toán cho ${ctv.name}`} sub={`${ctv.phone} · ${rows.length} hồ sơ đã duyệt`}>
        <Link className="btn btn-secondary" to={`/ctv/${ctv.id}`}>
          Hồ sơ CTV
        </Link>
      </PageHead>

      <div className="grid-2">
        <div className="card stack">
          <h2>Chuyển khoản</h2>
          <dl className="facts">
            <dt>Ngân hàng</dt>
            <dd>{bank.name || '—'}</dd>
            <dt>Số tài khoản</dt>
            <dd>
              {bank.no || '—'}{' '}
              {bank.no && (
                <button type="button" className="link-button" onClick={() => copy(bank.no)}>
                  Chép
                </button>
              )}
            </dd>
            <dt>Chủ tài khoản</dt>
            <dd>{bank.owner || ctv.bank || <span className="text-bad">CTV chưa nhập tài khoản</span>}</dd>
            <dt>Số tiền</dt>
            <dd>
              <strong className="big">{money(d.total)}</strong>{' '}
              <button type="button" className="link-button" onClick={() => copy(d.total)}>
                Chép
              </button>
            </dd>
            <dt>Nội dung</dt>
            <dd>
              {d.memo}{' '}
              <button type="button" className="link-button" onClick={() => copy(d.memo)}>
                Chép
              </button>
            </dd>
          </dl>
          <input className="input" placeholder="Mã giao dịch ngân hàng (nên ghi để đối soát)" value={ref} onChange={(e) => setRef(e.target.value)} />
          <Button onClick={pay} disabled={!enough} loading={busy === 'pay'}>
            Đã chuyển khoản {money(d.total)}
          </Button>
          {!enough && <p className="muted">Cần ít nhất {cfg?.PAY_MIN ?? 5} hồ sơ đã duyệt và tổng tiền lớn hơn 0đ.</p>}
        </div>
        <AdjustBox ctvId={ctv.id} adjs={adjs} act={act} busy={busy} ve="tt" />
      </div>

      <div className="card stack">
        <h2>Hồ sơ được trả lần này</h2>
        <HouseTable rows={rows} stats={d.stats} showCtv={false} />
      </div>
    </section>
  )
}
