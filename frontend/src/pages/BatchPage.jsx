// 1 phiếu chi (/dot/:id): số tiền, tài khoản nhận, mã giao dịch, biên lai, hồ sơ + thưởng/trừ, lịch sử.
// Kế toán: sửa mã giao dịch, tải ảnh biên lai (CTV thấy trên app), huỷ phiếu ghi nhầm.
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useAuth from '@/hooks/useAuth'
import { AuthImage, Button, HouseTable, PageHead, PageState, Pill } from '@/components/ui'
import { money, vnTime } from '@/utils/format'

export default function BatchPage() {
  const { id } = useParams()
  const { data: d, err, reload } = usePage(`/dot/${id}`)
  if (!d) return <PageState err={err} retry={reload} />
  return <Batch key={`${id}-${d.b.status}-${d.b.note}-${d.b.receipt}`} id={id} d={d} reload={reload} />
}

function Batch({ id, d, reload }) {
  const { role } = useAuth()
  const [act, busy] = useAct(reload)
  const { b } = d
  const kt = role === 'ketoan' && b.status === 'da_chi'
  const [ref, setRef] = useState(b.note || '')
  const [why, setWhy] = useState('')
  const [file, setFile] = useState(null)

  return (
    <section className="stack">
      <PageHead title={`Phiếu chi #${b.id}`} sub={vnTime(b.created_at)}>
        {b.status === 'huy' ? <Pill tone="bad">Đã huỷ</Pill> : <Pill tone="done">Đã thanh toán</Pill>}
      </PageHead>

      <div className="grid-2">
        <div className="card stack">
          <dl className="facts">
            <dt>Số tiền</dt>
            <dd>
              <strong className="big">{money(d.total)}</strong>
            </dd>
            <dt>Cộng tác viên</dt>
            <dd>
              {b.ctv_id ? <Link to={`/ctv/${b.ctv_id}`}>{b.ctv_name}</Link> : b.ctv_name} · {b.ctv_phone}
            </dd>
            <dt>Tài khoản nhận</dt>
            <dd>{b.ctv_bank || '—'}</dd>
            <dt>Mã giao dịch</dt>
            <dd>{b.note || <span className="muted">Không ghi</span>}</dd>
            <dt>Kế toán</dt>
            <dd>{b.by_name || '—'}</dd>
          </dl>
          {kt && (
            <form
              className="row-wrap"
              onSubmit={(e) => {
                e.preventDefault()
                act('ref', `/dot/${id}/sua`, { ref })
              }}
            >
              <input className="input grow" value={ref} onChange={(e) => setRef(e.target.value)} placeholder="Mã giao dịch" />
              <Button type="submit" variant="secondary" loading={busy === 'ref'}>
                Sửa mã
              </Button>
            </form>
          )}
        </div>

        <div className="card stack">
          <h2>Ảnh biên lai</h2>
          {b.receipt ? (
            <AuthImage key={b.receipt} path={`/dot/${b.id}/bien-lai?v=${b.receipt}`} alt="Biên lai chuyển khoản" className="receipt" />
          ) : (
            <p className="muted">Chưa có ảnh biên lai.</p>
          )}
          {kt && (
            <form
              className="row-wrap"
              onSubmit={(e) => {
                e.preventDefault()
                const fd = new FormData()
                fd.append('file', file)
                act('up', `/dot/${id}/bien-lai`, fd).then((j) => j && setFile(null))
              }}
            >
              <input type="file" accept="image/*" onChange={(e) => setFile(e.target.files?.[0] || null)} />
              <Button type="submit" variant="secondary" disabled={!file} loading={busy === 'up'}>
                {b.receipt ? 'Thay ảnh biên lai' : 'Tải ảnh biên lai'}
              </Button>
            </form>
          )}
        </div>
      </div>

      {d.adjs?.length > 0 && (
        <div className="card stack">
          <h2>Thưởng / trừ</h2>
          <ul className="list">
            {d.adjs.map((a) => (
              <li key={a.id} className="row-between">
                <span>{a.note}</span>
                <strong className={a.amount > 0 ? 'text-ok' : 'text-bad'}>
                  {a.amount > 0 ? '+' : ''}
                  {money(a.amount)}
                </strong>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="card stack">
        <h2>Hồ sơ ({d.rows?.length || 0})</h2>
        <HouseTable rows={d.rows || []} stats={d.stats} showCtv={false} />
      </div>

      {d.logs?.length > 0 && (
        <div className="card stack">
          <h2>Lịch sử</h2>
          <ul className="log">
            {d.logs.map((l) => (
              <li key={l.id}>
                <small className="muted">{vnTime(l.at)}</small>
                <span>
                  <strong>{l.action}</strong> · {l.name}
                </span>
                {l.note && <small className="muted">{l.note}</small>}
              </li>
            ))}
          </ul>
        </div>
      )}

      {kt && (
        <form
          className="card stack danger-zone"
          onSubmit={(e) => {
            e.preventDefault()
            if (window.confirm(`Huỷ phiếu chi #${b.id}? Hồ sơ về lại Chờ thanh toán.`)) act('cancel', `/dot/${id}/huy`, { reason: why })
          }}
        >
          <h2>Huỷ phiếu chi ghi nhầm</h2>
          <p className="muted">Hồ sơ về lại Chờ thanh toán, thưởng/trừ về chưa trả. Phiếu vẫn được giữ để đối soát.</p>
          <input className="input" placeholder="Lý do huỷ" value={why} onChange={(e) => setWhy(e.target.value)} />
          <Button type="submit" variant="secondary" className="btn-danger" disabled={!why.trim()} loading={busy === 'cancel'}>
            Huỷ phiếu chi
          </Button>
        </form>
      )}
    </section>
  )
}
