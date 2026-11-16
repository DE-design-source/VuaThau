// 1 hồ sơ (/houses/:id) cho admin, kế toán: ảnh + lỗi AI, thông tin công trình, tiến trình, lịch sử.
// Admin: duyệt / trả lại / từ chối, hoàn tác, sửa thông tin. Admin + kế toán: sửa tiền hồ sơ đã duyệt.
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useCfg from '@/hooks/useCfg'
import useAuth from '@/hooks/useAuth'
import houseApi from '@/api/houseApi'
import { Button, PageHead, PageState, PhotoGrid, Pill, StatusBadge } from '@/components/ui'
import { REASONS, bankParts, money, parseMoney, vnTime } from '@/utils/format'

export default function HousePage() {
  const { id } = useParams()
  const { data: d, err, reload } = usePage(() => houseApi.get(id), [id])
  if (!d) return <PageState err={err} retry={reload} />
  // Vừa sang hồ sơ khác (duyệt lần lượt): chờ dữ liệu hồ sơ mới
  if (String(d.house?.id) !== String(id)) return <PageState err={err} retry={reload} />
  return <House key={`${id}-${d.house.status}`} id={id} d={d} reload={reload} />
}

function House({ id, d, reload }) {
  const { role } = useAuth()
  const cfg = useCfg()
  const [act, busy] = useAct(reload)
  const h = d.house
  const ps = d.photos || []
  const admin = role === 'admin'
  const issues = d.issues || []
  // Điều còn thiếu ngoài ảnh (lỗi từng ảnh đã nằm trong issues)
  const fieldsTodo = (d.checklist || []).filter((c) => !c.ok && c.key !== 'anh')
  const bank = bankParts(h.ctv_bank)

  return (
    <section className="stack">
      <PageHead title={`Hồ sơ #${h.id}`} sub={h.address || 'Chưa có địa chỉ'}>
        <StatusBadge status={h.status} />
        {h.source === 'inbox' && <Pill tone="wait">Gửi qua Messenger</Pill>}
        {h.status === 'cho_duyet' && <span className="muted">Còn {d.queue_left} hồ sơ chờ duyệt</span>}
      </PageHead>

      {h.dup && !(h.status === 'tu_choi' && h.dup === h.note) && <div className="note tone-warn">⚠ {h.dup}</div>}
      {h.status === 'tra_lai' && h.note && <div className="note tone-warn">Đã trả lại cho CTV: {h.note}</div>}
      {h.status === 'tu_choi' && (
        <div className="note tone-bad">
          {h.dup_of ? 'Tự từ chối vì chụp trùng' : 'Đã từ chối'}: {h.note || 'không ghi lý do'}
          {h.dup_of && (
            <>
              {' '}
              <Link to={`/houses/${h.dup_of}`}>Xem hồ sơ #{h.dup_of} đã chụp trước</Link>
              {admin && '. AI nhầm thì bấm Hoàn tác để đưa về chờ duyệt.'}
            </>
          )}
        </div>
      )}

      <div className="grid-2">
        <div className="stack">
          <div className="card stack">
            <div className="row-between">
              <h2>
                Ảnh · {d.valid}/{ps.length} đạt
              </h2>
              <span className="muted">{h.ai_state === 'dang_doc' ? 'AI đang đọc ảnh…' : h.ai_state === 'loi' ? 'AI chưa đọc được chữ' : ''}</span>
            </div>
            {ps.length ? <PhotoGrid ps={ps} /> : <p className="muted">Chưa có ảnh.</p>}
            {(issues.length > 0 || fieldsTodo.length > 0) && (
              <ul className="issues">
                {issues.map((it) => (
                  <li key={it.index}>
                    Ảnh {it.index}: {(it.problems || []).join('; ')}
                  </li>
                ))}
                {fieldsTodo.map((c) => (
                  <li key={c.key}>{c.message || c.label}</li>
                ))}
              </ul>
            )}
          </div>

          {admin && h.status === 'cho_duyet' && <Review id={id} act={act} busy={busy} pay={d.pay} reasons={cfg?.reasons || REASONS} />}
          {admin && ['da_duyet', 'tra_lai', 'tu_choi'].includes(h.status) && (
            <div className="card row-between">
              <span className="muted">Duyệt / trả lại / từ chối nhầm?</span>
              <Button
                variant="secondary"
                loading={busy === 'undo'}
                onClick={() => window.confirm(`Đưa hồ sơ #${h.id} về Chờ duyệt?`) && act('undo', () => houseApi.undo(id))}
              >
                Hoàn tác về Chờ duyệt
              </Button>
            </div>
          )}
          {h.status === 'da_duyet' && <AmountForm id={id} act={act} busy={busy} amount={h.amount} />}
        </div>

        <div className="stack">
          <div className="card stack">
            <h2>Cộng tác viên</h2>
            <p>
              <Link to={`/ctvs/${h.ctv_id}`}>
                <strong>{h.ctv_name}</strong>
              </Link>{' '}
              · {h.ctv_phone}
            </p>
            <p className="muted">{bank.owner ? `${bank.name} · ${bank.no} · ${bank.owner}` : h.ctv_bank || 'Chưa có tài khoản ngân hàng'}</p>
            <div className="row-between">
              <span className="muted">Tiền hồ sơ</span>
              <strong className="big">{money(h.status === 'nhap' || h.status === 'tra_lai' ? d.pay : h.amount)}</strong>
            </div>
          </div>

          {admin && h.status !== 'da_chi' ? (
            <InfoForm id={id} h={h} act={act} busy={busy} stages={cfg?.stages || []} aiTag={d.ai_fields || []} />
          ) : (
            <div className="card stack">
              <h2>Thông tin công trình</h2>
              <Fact label="Địa chỉ" value={h.address}>
                {h.lat != null && (
                  <a href={`https://maps.google.com/?q=${h.lat},${h.lng}`} target="_blank" rel="noreferrer">
                    Xem bản đồ
                  </a>
                )}
              </Fact>
              <Fact label="Số tầng" value={h.floors} />
              <Fact label="Giai đoạn" value={h.stage} />
              <Fact label="Nhà thầu" value={[h.contractor_name, h.contractor_phone].filter(Boolean).join(' · ')} />
            </div>
          )}

          <div className="card stack">
            <h2>Tiến trình</h2>
            <ol className="steps">
              {(d.steps || []).map((s, k) => (
                <li key={k} className={`step step-${s.state}`}>
                  <strong>{s.title}</strong>
                  <small className="muted">
                    {s.who}
                    {s.at ? ` · ${vnTime(s.at)}` : ''}
                  </small>
                </li>
              ))}
            </ol>
          </div>

          {d.logs?.length > 0 && (
            <details className="card">
              <summary>
                <strong>Lịch sử ({d.logs.length})</strong>
              </summary>
              <ul className="log">
                {[...d.logs].reverse().map((l) => (
                  <li key={l.id}>
                    <small className="muted">{vnTime(l.at)}</small>
                    <span>
                      <strong>{l.action}</strong> · {l.user_name}
                    </span>
                    {l.note && <small className="muted">{l.note}</small>}
                  </li>
                ))}
              </ul>
            </details>
          )}
        </div>
      </div>
    </section>
  )
}

function Fact({ label, value, children }) {
  return (
    <div className="fact">
      <small className="muted">{label}</small>
      <span>{value || '—'}</span>
      {children}
    </div>
  )
}

function Review({ id, act, busy, pay, reasons }) {
  const [mode, setMode] = useState('') // '' | tra_lai | tu_choi
  const [reason, setReason] = useState('')
  // Duyệt xong sang hồ sơ chờ duyệt kế tiếp (next_id), hết thì về hàng chờ
  const send = (action) =>
    act(action, () => houseApi.review(id, { action, reason: action === 'duyet' ? '' : reason }), {
      go: (data) => (data.next_id ? `/houses/${data.next_id}` : '/queue'),
    })
  return (
    <div className="card stack review">
      <h2>Duyệt hồ sơ</h2>
      <p className="muted">Duyệt xong tự sang hồ sơ chờ duyệt kế tiếp. Tiền hồ sơ: {money(pay)} (tính theo số ảnh đạt).</p>
      <div className="row-wrap">
        <Button onClick={() => send('duyet')} loading={busy === 'duyet'} disabled={!!busy}>
          ✓ Duyệt
        </Button>
        <Button variant="secondary" onClick={() => setMode(mode === 'tra_lai' ? '' : 'tra_lai')}>
          Trả lại cho CTV sửa
        </Button>
        <Button variant="secondary" className="btn-danger" onClick={() => setMode(mode === 'tu_choi' ? '' : 'tu_choi')}>
          Từ chối
        </Button>
      </div>
      {mode && (
        <div className="stack">
          <div className="chips">
            {(reasons[mode] || REASONS[mode] || []).map((r) => (
              <button key={r} type="button" className="chip" onClick={() => setReason(reason ? `${reason}; ${r}` : r)}>
                {r}
              </button>
            ))}
          </div>
          <textarea className="input" rows={3} placeholder="Lý do (CTV sẽ thấy trên app)" value={reason} onChange={(e) => setReason(e.target.value)} />
          <Button onClick={() => send(mode)} loading={busy === mode} disabled={!reason.trim() || !!busy}>
            {mode === 'tra_lai' ? 'Gửi trả lại' : 'Xác nhận từ chối'}
          </Button>
        </div>
      )}
    </div>
  )
}

function InfoForm({ id, h, act, busy, stages, aiTag }) {
  const [f, setF] = useState({
    address: h.address || '',
    floors: h.floors ?? '',
    stage: h.stage || '',
    contractor_name: h.contractor_name || '',
    contractor_phone: h.contractor_phone || '',
  })
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value })
  const ai = (k) => aiTag.includes(k) && <Pill tone="warn">AI đọc</Pill>
  return (
    <form
      className="card stack"
      onSubmit={(e) => {
        e.preventDefault()
        const fl = String(f.floors).trim()
        act('info', () => houseApi.updateInfo(id, { ...f, floors: fl === '' ? null : Number(fl) }))
      }}
    >
      <h2>Thông tin công trình</h2>
      <label className="field">
        <span>Địa chỉ {ai('address')}</span>
        <textarea className="input" rows={2} value={f.address} onChange={set('address')} />
        {h.lat != null && (
          <a href={`https://maps.google.com/?q=${h.lat},${h.lng}`} target="_blank" rel="noreferrer">
            Xem vị trí trên bản đồ
          </a>
        )}
      </label>
      <div className="grid-2 tight">
        <label className="field">
          <span>Số tầng {ai('floors')}</span>
          <input className="input" inputMode="numeric" value={f.floors} onChange={set('floors')} />
        </label>
        <label className="field">
          <span>Giai đoạn {ai('stage')}</span>
          <select className="input" value={f.stage} onChange={set('stage')}>
            <option value="">—</option>
            {stages.map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Tên nhà thầu {ai('contractor_name')}</span>
          <input className="input" value={f.contractor_name} onChange={set('contractor_name')} />
        </label>
        <label className="field">
          <span>SĐT nhà thầu {ai('contractor_phone')}</span>
          <input className="input" inputMode="tel" value={f.contractor_phone} onChange={set('contractor_phone')} />
        </label>
      </div>
      <Button type="submit" variant="secondary" loading={busy === 'info'}>
        Lưu thông tin (ghi lịch sử)
      </Button>
    </form>
  )
}

function AmountForm({ id, act, busy, amount }) {
  const [v, setV] = useState(String(amount ?? ''))
  const [reason, setReason] = useState('')
  return (
    <form
      className="card stack"
      onSubmit={(e) => {
        e.preventDefault()
        act('amount', () => houseApi.setAmount(id, { amount: parseMoney(v), reason }))
      }}
    >
      <h2>Sửa số tiền hồ sơ</h2>
      <p className="muted">Chỉ hồ sơ đã duyệt, chưa thanh toán. Bắt buộc ghi lý do.</p>
      <div className="grid-2 tight">
        <input className="input" inputMode="numeric" value={v} onChange={(e) => setV(e.target.value)} aria-label="Số tiền" />
        <input className="input" placeholder="Lý do" value={reason} onChange={(e) => setReason(e.target.value)} />
      </div>
      <Button type="submit" variant="secondary" loading={busy === 'amount'} disabled={!reason.trim() || Number.isNaN(parseMoney(v))}>
        Lưu số tiền
      </Button>
    </form>
  )
}
