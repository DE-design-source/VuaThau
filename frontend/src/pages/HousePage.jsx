// 1 hồ sơ (/houses/:id) cho admin, kế toán, theo thiết kế 18/11/2026 (kiểu trang chi tiết Airbnb):
//   trái: mã + trạng thái + AI verified, địa chỉ, giai đoạn; ảnh khảm (1 lớn + 4 nhỏ, xem tất cả); nhật ký công trình 8 bước;
//         tiền hồ sơ, chuyển khoản cho CTV, tiến trình duyệt; lịch sử.
//   phải: nút Trả lại / Từ chối / Duyệt hồ sơ (+ hồ sơ kế tiếp) hoặc Hoàn tác; thông tin công trình (sửa được); bản đồ.
// Admin: duyệt / trả lại / từ chối, hoàn tác, sửa thông tin. Admin + kế toán: sửa tiền hồ sơ đã duyệt.
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useCfg from '@/hooks/useCfg'
import useAuth from '@/hooks/useAuth'
import houseApi from '@/api/houseApi'
import { AuthImage, Button, Icon, PageState, PhotoGrid, StatusBadge, hsId } from '@/components/ui'
import { toast } from '@/services/toast'
import { REASONS, aiDiffText, bankParts, copyGmap, gmapUrl, money, parseMoney, vnTime } from '@/utils/format'

// Mô tả 8 giai đoạn (hiện khi rê chuột ở nhật ký công trình)
const STAGE_DESC = {
  'Chuẩn bị mặt bằng': 'Giấy phép, dọn dẹp, định vị tim mốc',
  'Ép cọc, đào móng': 'Ép cọc bê tông, đào hố móng',
  'Thi công móng': 'Móng, đà kiềng, hầm phân',
  'Xây phần thô': 'Cột, dầm, sàn từng tầng',
  'Xây tường, đổ mái': 'Tường bao, cầu thang, mái',
  'Điện nước âm tường': 'Đi ống, dây, chống thấm',
  'Hoàn thiện': 'Trát, ốp lát, sơn bả, cửa',
  'Bàn giao': 'Nghiệm thu, vệ sinh, bàn giao',
}

// Gộp lỗi từng ảnh theo loại lỗi: "Không thấy công trình đang xây: ảnh 3, 5"
function groupIssues(issues) {
  const m = new Map()
  for (const it of issues) for (const p of it.problems || []) m.set(p, [...(m.get(p) || []), it.index])
  return [...m]
}

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
  const [allPhotos, setAllPhotos] = useState(false)
  const [showLogs, setShowLogs] = useState(false)
  const h = d.house
  const ps = d.photos || []
  const admin = role === 'admin'
  const issues = d.issues || []
  const fieldsTodo = (d.checklist || []).filter((c) => !c.ok && c.key !== 'anh')
  const bank = bankParts(h.ctv_bank)
  const stages = cfg?.stages || []
  const amount = h.status === 'nhap' || h.status === 'tra_lai' ? d.pay : h.amount

  return (
    <section className="hp">
      <div className="hp-cols">
        {/* ---------------- cột trái ---------------- */}
        <div className="hp-left">
          <div className="hp-head">
            <div className="hp-head-main">
              <div className="hp-meta">
                <span>
                  Mã số <b>{hsId(h.id)}</b>
                </span>
                <StatusBadge status={h.status} />
                <span className={`hp-ai ${d.valid === ps.length && ps.length ? '' : 'warn'}`}>
                  <i>{d.valid === ps.length && ps.length ? '✓' : '!'}</i>
                  <span>AI verified</span>
                  <em />
                  <span>
                    <b>
                      {d.valid}/{ps.length}
                    </b>{' '}
                    ảnh đạt
                  </span>
                </span>
                {h.source === 'inbox' && <span className="hp-src">Messenger</span>}
                {h.ai_state === 'dang_doc' && <span className="muted">AI đang đọc ảnh…</span>}
              </div>
              <h1>{h.address || 'Chưa có địa chỉ'}</h1>
              <div className="hp-stage-line">
                <span>Đang thi công tới bước</span>
                <span className="hp-stage">
                  <i />
                  {h.stage || 'Chưa rõ'}
                </span>
                {h.province_name && <span className="muted">· {h.province_name}</span>}
              </div>
            </div>
            <Link to={`/ctvs/${h.ctv_id}`} className="hp-ctv">
              <span className="hp-avatar">
                {(h.ctv_name || '?').trim().split(/\s+/).pop()?.[0]?.toUpperCase() || '?'}
                {h.vip && <em title="CTV chụp công trình này đầu tiên">VIP</em>}
              </span>
              <span>
                <small>CTV</small>
                <b>{h.ctv_name}</b>
              </span>
            </Link>
          </div>

          {h.dup && !(h.status === 'tu_choi' && h.dup === h.note) && (
            <div className="note tone-warn inline">
              <Icon name="alert" size={16} /> {h.dup}
            </div>
          )}
          {h.ai_diff && (
            <div className="note tone-warn inline">
              <Icon name="alert" size={16} /> AI đọc trên ảnh khác {aiDiffText(h.ai_diff)} đang lưu. Xem giá trị AI đọc ở phần Thông tin công trình.
            </div>
          )}
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

          {/* ảnh: khảm (1 lớn + 4 nhỏ) hoặc tất cả */}
          {ps.length === 0 ? (
            <p className="muted">Chưa có ảnh.</p>
          ) : allPhotos || ps.length < 3 ? (
            <div className="stack">
              <PhotoGrid ps={ps} />
              {ps.length >= 3 && (
                <a href="#thu-gon" className="hp-link" onClick={(e) => (e.preventDefault(), setAllPhotos(false))}>
                  Thu gọn
                </a>
              )}
            </div>
          ) : (
            <div className="hp-mosaic">
              {ps.slice(0, 5).map((p, k) => (
                <button key={p.id} type="button" className={`hp-tile ${k === 0 ? 'hp-big' : ''}`} onClick={() => setAllPhotos(true)}>
                  <AuthImage path={`/photos/${p.id}`} alt={`Ảnh ${k + 1}`} />
                  {k === 0 ? (
                    <span className={`hp-first ${p.problems?.length ? 'bad' : ''}`}>
                      <i>{p.problems?.length ? '✕' : '✓'}</i>Ảnh 1 · {p.problems?.length ? 'Lỗi' : 'Đạt'}
                    </span>
                  ) : (
                    <span className={`hp-mark ${p.problems?.length ? 'bad' : ''}`}>{p.problems?.length ? '✕' : '✓'}</span>
                  )}
                </button>
              ))}
              <button type="button" className="hp-all" onClick={() => setAllPhotos(true)}>
                <svg width="14" height="14" viewBox="0 0 16 16" fill="#222" aria-hidden="true">
                  {[2, 8, 14].flatMap((y) => [2, 8, 14].map((x) => <circle key={`${x}${y}`} cx={x} cy={y} r="1.5" />))}
                </svg>
                Hiển thị tất cả {ps.length} ảnh
              </button>
            </div>
          )}
          {(issues.length > 0 || fieldsTodo.length > 0) && (
            <ul className="issues">
              {groupIssues(issues).map(([problem, idx]) => (
                <li key={problem}>
                  {problem}: ảnh {idx.join(', ')}
                </li>
              ))}
              {fieldsTodo.map((c) => (
                <li key={c.key}>{c.message || c.label}</li>
              ))}
            </ul>
          )}

          {/* nhật ký công trình: 8 giai đoạn */}
          {stages.length > 0 && <Milestones stages={stages} current={stages.indexOf(h.stage)} journal={d.journal || []} />}

          {/* tiền + chuyển khoản + tiến trình */}
          <div className="hp-card">
            <div className="hp-money">
              <div className="hp-price">
                <b>{money(amount)}</b>
                <span>tiền hồ sơ</span>
              </div>
              <div className="hp-box hp-box-2">
                <div>
                  <span className="cap">Ảnh đạt</span>
                  <div>
                    {d.valid}/{ps.length}
                  </div>
                </div>
                <div>
                  <span className="cap">Giai đoạn</span>
                  <div>{h.stage || '—'}</div>
                </div>
              </div>
            </div>
            <hr />
            <div className="stack-sm">
              <span className="cap">Chuyển khoản cho cộng tác viên</span>
              <div className="hp-box hp-box-3">
                <div>
                  <span className="cap">Ngân hàng</span>
                  <div>{bank.name || '—'}</div>
                </div>
                <div>
                  <span className="cap">Chủ tài khoản</span>
                  <div>{bank.owner || (h.ctv_bank ? '—' : <span className="muted">Chưa có tài khoản</span>)}</div>
                </div>
                <div>
                  <span className="cap">Số tài khoản</span>
                  <div className="mono">{bank.no || '—'}</div>
                </div>
              </div>
              {h.ctv_phone && <small className="muted">SĐT cộng tác viên: {h.ctv_phone}</small>}
            </div>
            <hr />
            <div className="stack-sm">
              <span className="cap">Tiến trình duyệt</span>
              <div className="hp-steps">
                {(d.steps || []).map((s, k) => (
                  <div key={k} className={`hp-step ${s.state}`} title={s.hint || ''}>
                    <i />
                    <b>{s.title}</b>
                    <small>
                      {s.who}
                      {s.at ? ` · ${vnTime(s.at)}` : ''}
                    </small>
                  </div>
                ))}
              </div>
            </div>
          </div>

          {h.status === 'da_duyet' && <AmountForm id={id} act={act} busy={busy} amount={h.amount} />}

          {d.logs?.length > 0 && (
            <div className="hp-logs">
              <a href="#lich-su" className="hp-link muted" onClick={(e) => (e.preventDefault(), setShowLogs(!showLogs))}>
                {showLogs ? 'Ẩn lịch sử chỉnh sửa' : `Xem lịch sử chỉnh sửa (${d.logs.length})`}
              </a>
              {showLogs && (
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
              )}
            </div>
          )}
        </div>

        {/* ---------------- cột phải ---------------- */}
        <div className="hp-right">
          <div className="hp-actions">
            {admin && h.status === 'cho_duyet' && <Review id={id} act={act} busy={busy} left={d.queue_left} reasons={cfg?.reasons || REASONS} />}
            {admin && ['da_duyet', 'tra_lai', 'tu_choi'].includes(h.status) && (
              <div className="hp-done">
                <div>
                  <b>{{ da_duyet: 'Đã duyệt hồ sơ', tra_lai: 'Đã trả lại cho CTV', tu_choi: 'Đã từ chối hồ sơ' }[h.status]}</b>
                  <small>
                    {{
                      da_duyet: `Chuyển kế toán thanh toán ${money(h.amount)} cho ${h.ctv_name}.`,
                      tra_lai: `${h.ctv_name} sẽ thấy lý do trên app.`,
                      tu_choi: 'Hồ sơ không được tính tiền.',
                    }[h.status]}
                  </small>
                </div>
                <a
                  href="#hoan-tac"
                  onClick={(e) => {
                    e.preventDefault()
                    if (window.confirm(`Đưa hồ sơ ${hsId(h.id)} về Chờ duyệt?`)) act('undo', () => houseApi.undo(id))
                  }}
                >
                  Hoàn tác
                </a>
              </div>
            )}
            {!admin && <StatusBadge status={h.status} />}
          </div>

          <InfoForm id={id} h={h} act={act} busy={busy} stages={stages} aiTag={d.ai_fields || []} editable={admin && h.status !== 'da_chi'} />
        </div>
      </div>
    </section>
  )
}

/** Nhật ký công trình: 8 giai đoạn, đã qua (đặc), đang thi công (viền xanh), chưa tới (xám).
 *  Bước có bộ ảnh đã duyệt (cùng công trình) bấm được: hiện các bộ ảnh của bước đó bên dưới. */
function Milestones({ stages, current, journal }) {
  const [pick, setPick] = useState(null)
  const byStage = (s) => journal.filter((j) => j.stage === s)
  const sets = pick === null ? [] : byStage(stages[pick])
  const other = journal.filter((j) => !stages.includes(j.stage)) // hồ sơ chưa ghi giai đoạn
  return (
    <div className="hp-card">
      <div className="hp-ms-head">
        <h3 className="hp-h3">Xem ảnh nhật ký công trình</h3>
        <small className="muted">{journal.length > 1 ? `${journal.length} bộ ảnh của công trình` : ''}</small>
      </div>
      <div className="hp-ms" style={{ gridTemplateColumns: `repeat(${stages.length}, minmax(0, 1fr))` }}>
        {stages.map((s, i) => {
          const done = i < current
          const cur = i === current
          const n = byStage(s).length
          return (
            <button
              type="button"
              key={s}
              disabled={!n}
              aria-pressed={pick === i}
              onClick={() => setPick(pick === i ? null : i)}
              className={`hp-ms-item ${done ? 'done' : ''} ${cur ? 'cur' : ''} ${i > current ? 'todo' : ''} ${n ? 'has' : ''} ${pick === i ? 'on' : ''}`}
              title={(STAGE_DESC[s] || '') + (n ? ` · bấm xem ${n} bộ ảnh` : '')}
            >
              <div className="hp-ms-track">
                <span className="hp-ms-dot">{done ? '✓' : ''}</span>
                {i < stages.length - 1 && <span className="hp-ms-line" />}
              </div>
              <div className="hp-ms-title">{s}</div>
              {cur && <span className="hp-ms-now">Đang thi công</span>}
              {n > 0 && <span className="hp-ms-n">{n} bộ ảnh</span>}
            </button>
          )
        })}
      </div>
      {pick === null ? (
        <p className="muted hp-ms-hint">
          {journal.some((j) => stages.includes(j.stage)) ? 'Bấm vào 1 bước có ảnh để xem lại bộ ảnh đã duyệt của bước đó.' : 'Công trình chưa có bộ ảnh nào đã duyệt.'}
          {other.length > 0 && ` ${other.length} bộ ảnh chưa ghi giai đoạn.`}
        </p>
      ) : (
        <div className="hp-sets">
          {sets.map((j) => (
            <div key={j.house_id} className="hp-set">
              <div className="hp-set-head">
                <b>{stages[pick]}</b>
                <span className="muted">
                  {vnTime(j.at)}
                  {j.ctv ? ` · ${j.ctv}` : ''}
                </span>
                <span className="spacer" />
                {j.self ? <span className="hp-src">Hồ sơ đang xem</span> : <Link to={`/houses/${j.house_id}`}>{hsId(j.house_id)}</Link>}
              </div>
              {j.photos.length ? <PhotoGrid ps={j.photos.map((id) => ({ id }))} checks={false} /> : <p className="muted">Chưa có ảnh.</p>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// Giá trị AI đọc trên ảnh khi khác giá trị đang lưu; use: bấm để điền vào ô.
function AIRead({ v, use }) {
  if (v == null || v === '') return null
  return (
    <small className="text-warn inline ai-read">
      <Icon name="alert" size={13} /> AI đọc trên ảnh: <b>{v}</b>
      {use && (
        <button type="button" className="link-btn" onClick={() => use(String(v))}>
          Dùng giá trị này
        </button>
      )}
    </small>
  )
}

function Review({ id, act, busy, left, reasons }) {
  const [mode, setMode] = useState('') // '' | tra_lai | tu_choi
  const [reason, setReason] = useState('')
  // Duyệt xong sang hồ sơ chờ duyệt kế tiếp (next_id), hết thì về hàng chờ
  const send = (action) =>
    act(action, () => houseApi.review(id, { action, reason: action === 'duyet' ? '' : reason }), {
      go: (data) => (data.next_id ? `/houses/${data.next_id}` : '/queue'),
    })
  return (
    <div className="hp-review">
      <div className="hp-review-row">
        <Button variant="secondary" onClick={() => setMode(mode === 'tra_lai' ? '' : 'tra_lai')}>
          Trả lại CTV sửa
        </Button>
        <Button variant="secondary" className="btn-danger" onClick={() => setMode(mode === 'tu_choi' ? '' : 'tu_choi')}>
          Từ chối
        </Button>
        <Button onClick={() => send('duyet')} loading={busy === 'duyet'} disabled={!!busy}>
          Duyệt hồ sơ
        </Button>
        <div className="hp-next">
          <small>Còn {left} hồ sơ chờ duyệt</small>
          <Link to="/queue">
            Hồ sơ kế tiếp
            <svg width="12" height="12" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="4" aria-hidden="true">
              <path d="M12 4l12 12-12 12" />
            </svg>
          </Link>
        </div>
      </div>
      {mode && (
        <div className="hp-reason">
          <div className="chips">
            {(reasons[mode] || REASONS[mode] || []).map((r) => (
              <button key={r} type="button" className="chip" onClick={() => setReason(reason ? `${reason}; ${r}` : r)}>
                {r}
              </button>
            ))}
          </div>
          <textarea className="input" rows={3} placeholder="Lý do (CTV sẽ thấy trên app)" value={reason} onChange={(e) => setReason(e.target.value)} />
          <Button variant={mode === 'tu_choi' ? 'secondary' : 'primary'} className={mode === 'tu_choi' ? 'btn-danger' : ''} onClick={() => send(mode)} loading={busy === mode} disabled={!reason.trim() || !!busy}>
            {mode === 'tra_lai' ? 'Gửi trả lại' : 'Xác nhận từ chối'}
          </Button>
        </div>
      )}
    </div>
  )
}

/** Thông tin công trình: khung ô kiểu Airbnb; admin sửa được (ghi lịch sử), vai trò khác chỉ xem. */
function InfoForm({ id, h, act, busy, stages, aiTag, editable }) {
  const [f, setF] = useState({
    address: h.address || '',
    floors: h.floors ?? '',
    stage: h.stage || '',
    contractor_name: h.contractor_name || '',
    contractor_phone: h.contractor_phone || '',
  })
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value })
  const ai = (k) => aiTag.includes(k) && <em className="hp-aitag">AI đọc</em>
  // AI đọc khác giá trị đang lưu; ẩn khi ô đã được sửa thành đúng giá trị AI
  const diff = (k) => {
    const v = h.ai_diff?.[k]
    if (v == null || String(f[k]) === String(v)) return null
    return <AIRead v={v} use={editable ? (x) => setF((o) => ({ ...o, [k]: x })) : null} />
  }
  return (
    <form
      className="hp-info"
      onSubmit={(e) => {
        e.preventDefault()
        const fl = String(f.floors).trim()
        act('info', () => houseApi.updateInfo(id, { ...f, floors: fl === '' ? null : Number(fl) }))
      }}
    >
      <h3 className="hp-h3 hp-h3-lg">Thông tin công trình</h3>
      <div className="hp-fs">
        <label className="hp-cell hp-cell-hero">
          <span className="cap">Tên nhà thầu {ai('contractor_name')}</span>
          <textarea rows={2} value={f.contractor_name} onChange={set('contractor_name')} readOnly={!editable} placeholder="—" />
          {diff('contractor_name')}
        </label>
        <label className="hp-cell">
          <span className="cap">Địa chỉ công trình {ai('address')}</span>
          <textarea rows={2} value={f.address} onChange={set('address')} readOnly={!editable} placeholder="—" />
        </label>
        <div className="hp-cell-row">
          <label className="hp-cell">
            <span className="cap">SĐT nhà thầu {ai('contractor_phone')}</span>
            <input value={f.contractor_phone} onChange={set('contractor_phone')} readOnly={!editable} inputMode="tel" placeholder="—" />
            {diff('contractor_phone')}
          </label>
          <label className="hp-cell">
            <span className="cap">Số tầng {ai('floors')}</span>
            <input value={f.floors} onChange={set('floors')} readOnly={!editable} inputMode="numeric" placeholder="—" />
            {diff('floors')}
          </label>
        </div>
        <label className="hp-cell">
          <span className="cap">Giai đoạn thi công {ai('stage')}</span>
          {editable ? (
            <select value={f.stage} onChange={set('stage')}>
              <option value="">—</option>
              {stages.map((s) => (
                <option key={s}>{s}</option>
              ))}
              {f.stage && !stages.includes(f.stage) && <option>{f.stage}</option>}
            </select>
          ) : (
            <input value={f.stage || '—'} readOnly />
          )}
          {diff('stage')}
        </label>
      </div>
      <div className="hp-info-foot">
        {h.lat != null && (
          <>
            <a href={gmapUrl(h.lat, h.lng)} target="_blank" rel="noreferrer" className="hp-gmap">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
                <path d="M8 15s5-4.6 5-8.5A5 5 0 0 0 3 6.5C3 10.4 8 15 8 15z" />
                <circle cx="8" cy="6.5" r="1.8" />
              </svg>
              Xem trên Google Maps
            </a>
            <button type="button" className="hp-copy" title="Chép link Google Maps" onClick={() => copyGmap(h.lat, h.lng, toast)}>
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
                <rect x="5" y="5" width="9" height="9" rx="1.5" />
                <path d="M11 2.5H3.5a1 1 0 0 0-1 1V11" />
              </svg>
            </button>
          </>
        )}
        <span className="spacer" />
        {editable && (
          <Button type="submit" variant="secondary" loading={busy === 'info'}>
            Lưu thông tin
          </Button>
        )}
      </div>
      {h.lat != null && (
        <div className="hp-map">
          <iframe
            title="Bản đồ công trình"
            loading="lazy"
            referrerPolicy="no-referrer-when-downgrade"
            src={`https://maps.google.com/maps?q=${h.lat},${h.lng}&z=16&output=embed`}
          />
        </div>
      )}
    </form>
  )
}

function AmountForm({ id, act, busy, amount }) {
  const [v, setV] = useState(String(amount ?? ''))
  const [reason, setReason] = useState('')
  return (
    <form
      className="hp-card stack"
      onSubmit={(e) => {
        e.preventDefault()
        act('amount', () => houseApi.setAmount(id, { amount: parseMoney(v), reason }))
      }}
    >
      <h3 className="hp-h3">Sửa số tiền hồ sơ</h3>
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
