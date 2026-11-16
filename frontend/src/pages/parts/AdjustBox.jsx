// Thưởng (+) / trừ (-) tiền cho CTV, cộng vào lần thanh toán tới. Dùng ở trang CTV và trang Thanh toán.
import { useState } from 'react'
import { Button } from '@/components/ui'
import { money, vnDate } from '@/utils/format'

export default function AdjustBox({ ctvId, adjs, act, busy, ve = '' }) {
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  return (
    <div className="card stack">
      <h2>Thưởng / trừ tiền</h2>
      {adjs.length ? (
        <ul className="list">
          {adjs.map((a) => (
            <li key={a.id} className="row-between">
              <span>
                <strong className={a.amount > 0 ? 'text-ok' : 'text-bad'}>
                  {a.amount > 0 ? '+' : ''}
                  {money(a.amount)}
                </strong>{' '}
                {a.note}
                <small className="muted d-block">
                  {vnDate(a.created_at)}
                  {a.by_name ? ` · ${a.by_name}` : ''}
                  {a.batch_id ? ` · đã trả ở phiếu chi #${a.batch_id}` : ' · chờ trả'}
                </small>
              </span>
              {!a.batch_id && (
                <Button
                  variant="ghost"
                  loading={busy === `del${a.id}`}
                  onClick={() => window.confirm('Xoá khoản này?') && act(`del${a.id}`, `/dieu-chinh/${a.id}/xoa`, { ve })}
                >
                  Xoá
                </Button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="muted">Chưa có khoản nào.</p>
      )}
      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault()
          act('adj', `/ctv/${ctvId}/dieu-chinh`, { amount, reason, ve }).then((j) => {
            if (j) {
              setAmount('')
              setReason('')
            }
          })
        }}
      >
        <div className="grid-2 tight">
          <input className="input" inputMode="numeric" placeholder="Số tiền: 20000 hoặc -20000" value={amount} onChange={(e) => setAmount(e.target.value)} />
          <input className="input" placeholder="Lý do" value={reason} onChange={(e) => setReason(e.target.value)} />
        </div>
        <Button type="submit" variant="secondary" disabled={!amount || !reason.trim()} loading={busy === 'adj'}>
          Thêm khoản thưởng / trừ
        </Button>
      </form>
    </div>
  )
}
