// 1 hồ sơ (/houses/:id) cho admin, kế toán, theo thiết kế 18/11/2026 (kiểu trang chi tiết Airbnb):
//   trái: mã + trạng thái + AI verified, địa chỉ, giai đoạn; ảnh khảm (1 lớn + 4 nhỏ, xem tất cả); nhật ký công trình 8 bước;
//         tiền hồ sơ, chuyển khoản cho CTV, tiến trình duyệt; lịch sử.
//   phải: nút Trả lại / Từ chối / Duyệt hồ sơ (+ hồ sơ kế tiếp) hoặc Hoàn tác; thông tin công trình (sửa được); bản đồ.
// Admin: duyệt / trả lại / từ chối, hoàn tác, sửa thông tin, tách ảnh công trình khác (chatbot gộp nhầm) ra hồ sơ mới. Admin + kế toán: sửa tiền hồ sơ đã duyệt.
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useCfg from '@/hooks/useCfg'
import useAuth from '@/hooks/useAuth'
import houseApi from '@/api/houseApi'
import { AuthImage, Button, Icon, Lightbox, PageState, PhotoGrid, StatusBadge, hsId } from '@/components/ui'
import { toast } from '@/services/toast'
import { REASONS, aiDiffText, bankParts, copyGmap, gmapUrl, money, parseMoney, vnDate, vnTime } from '@/utils/format'

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
  const [showLogs, setShowLogs] = useState(false)
  const [asMasked, setAsMasked] = useState(false) // xem ảnh như sale / CTV thấy (SĐT đã che)
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
                <span className="hp-dot">·</span>
                <span>Cập nhật {vnDate(h.submitted_at || h.created_at)}</span>
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

          {/* ảnh + video: khảm theo hàng 3 ô (ô rộng theo tỉ lệ ảnh), 2 hàng đầu; bấm nút xem tất cả */}
          {ps.length + (d.videos || []).length === 0 ? <p className="muted">Chưa có ảnh.</p> : <Mosaic ps={ps} videos={d.videos || []} masked={asMasked} />}
          {ps.some((p) => p.masked > 0) && (
            <p className="hp-masked muted">
              Đã tự che SĐT trên {ps.filter((p) => p.masked > 0).length} ảnh khi sale, CTV, Google Drive xem (bạn đang xem {asMasked ? 'bản đã che' : 'ảnh gốc'}).{' '}
              <button type="button" className="link-btn" onClick={() => setAsMasked(!asMasked)}>
                {asMasked ? 'Xem ảnh gốc' : 'Xem bản đã che'}
              </button>
            </p>
          )}
          {admin && h.status === 'cho_duyet' && ps.length > 1 && <Split key={ps.length} id={id} ps={ps} videos={d.videos || []} act={act} busy={busy} />}
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
                {(d.videos || []).length > 0 && d.video_bonus > 0 && amount > 0 && <small className="muted">gồm thưởng video {money(d.video_bonus)}</small>}
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
            <Scope h={h} payScope={d.pay_scope} editable={admin && ['cho_duyet', 'da_duyet'].includes(h.status)} act={act} busy={busy} id={id} />
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
                  <span className="cap">Số điện thoại</span>
                  <div>{h.ctv_phone || '—'}</div>
                </div>
              </div>
              {bank.no && bank.no.replace(/\D/g, '') !== (h.ctv_phone || '').replace(/\D/g, '') &&<small className="muted">Số tài khoản: {bank.no}</small>}
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

const SCOPES = [
  ['ngoai', 'Chỉ chụp ngoài'],
  ['trong_ngoai', 'Chụp trong + ngoài'],
]

/** Loại chụp: chỉ bên ngoài công trình / cả trong lẫn ngoài (tiền khác nhau). AI gợi ý theo ảnh, admin chọn khi duyệt. */
function Scope({ id, h, payScope = {}, editable, act, busy }) {
  // hồ sơ Messenger chưa ai chọn: tính như chỉ chụp ngoài (khớp máy chủ)
  const cur = h.scope || h.scope_ai || (h.source === 'inbox' ? 'ngoai' : '')
  const ai = h.scope_ai && SCOPES.find(([k]) => k === h.scope_ai)?.[1]
  if (!cur && !editable) return null // hồ sơ cũ: tính tiền như trước
  return (
    <div className="hp-scope">
      <span className="cap">Loại chụp</span>
      <div className="hp-seg" role="radiogroup" aria-label="Loại chụp">
        {SCOPES.map(([k, label]) => (
          <button
            key={k}
            type="button"
            role="radio"
            aria-checked={cur === k}
            className={cur === k ? 'on' : ''}
            disabled={!editable || busy === 'scope'}
            onClick={() => cur !== k && act('scope', () => houseApi.setScope(id, k))}
          >
            <b>{label}</b>
            {payScope[k] != null && <small>{money(payScope[k])}</small>}
          </button>
        ))}
      </div>
      <small className="muted">
        {ai ? `AI gợi ý: ${ai}` : h.source === 'inbox' ? 'Hồ sơ Messenger không qua AI, mặc định chỉ chụp ngoài' : 'AI chưa gợi ý'}
        {h.scope && h.scope_ai && h.scope !== h.scope_ai ? ' · admin đã chọn khác' : ''}
      </small>
    </div>
  )
}

const ROW = 3 // ô mỗi hàng
const SHOW_ROWS = 2 // số hàng khi thu gọn

/** Ảnh khảm (thiết kế 19/11): hàng 3 ô cao bằng nhau, ô rộng theo tỉ lệ thật của ảnh / video (đo khi tải xong).
 *  Video đứng sau ảnh 1. Ảnh 1 có nhãn "Ảnh 1 · Đạt", ảnh khác có dấu ✓ / ✕. Bấm ảnh: xem lớn; bấm video: phát. */
/** Tách hồ sơ: chatbot Messenger gắn ảnh nhiều công trình vào 1 hồ sơ. Admin chọn ảnh / video của công trình khác,
 *  nhập địa chỉ -> hồ sơ mới cùng CTV (chờ duyệt), tiền 2 hồ sơ tính lại. */
function Split({ id, ps, videos, act, busy }) {
  const [open, setOpen] = useState(false)
  const [pick, setPick] = useState({}) // 'p12' | 'v3' -> true
  const [addr, setAddr] = useState('')
  const [made, setMade] = useState(null) // mã hồ sơ vừa tách
  const [at, setAt] = useState(null)
  const toggle = (k) => setPick((x) => ({ ...x, [k]: !x[k] }))
  const photos = ps.filter((p) => pick[`p${p.id}`]).map((p) => p.id)
  const vids = videos.filter((v) => pick[`v${v.id}`]).map((v) => v.id)
  const all = photos.length === ps.length
  if (!open)
    return (
      <div className="hp-split-open">
        <Button variant="secondary" onClick={() => setOpen(true)}>
          Tách hồ sơ
        </Button>
        <small className="muted">Hồ sơ có ảnh của công trình khác (chatbot gộp nhầm)? Tách ra hồ sơ mới.</small>
        {made && <Link to={`/houses/${made}`}>Mở hồ sơ vừa tách #{made}</Link>}
      </div>
    )
  return (
    <form
      className="card stack hp-split"
      onSubmit={(e) => {
        e.preventDefault()
        if (!window.confirm(`Tách ${photos.length} ảnh${vids.length ? `, ${vids.length} video` : ''} sang hồ sơ mới tại "${addr.trim()}"?`)) return
        act('split', () => houseApi.split(id, { photos, videos: vids, address: addr })).then((j) => j?.new_id && setMade(j.new_id))
      }}
    >
      <div className="row-between">
        <h2>Tách hồ sơ</h2>
        <Button variant="ghost" type="button" onClick={() => setOpen(false)}>
          Đóng
        </Button>
      </div>
      <p className="muted">Bấm chọn các ảnh{videos.length ? ' / video' : ''} của công trình khác. Bấm ⌕ ở góc ảnh để xem lớn.</p>
      <div className="hp-split-grid">
        {ps.map((p, i) => (
          <label key={p.id} className={`hp-split-tile${pick[`p${p.id}`] ? ' on' : ''}`}>
            <input type="checkbox" checked={!!pick[`p${p.id}`]} onChange={() => toggle(`p${p.id}`)} />
            <AuthImage path={`/photos/${p.id}`} alt={`Ảnh ${i + 1}`} />
            <span>{i + 1}</span>
            <button type="button" className="hp-split-zoom" aria-label={`Xem lớn ảnh ${i + 1}`} onClick={(e) => (e.preventDefault(), setAt(i))}>
              ⌕
            </button>
          </label>
        ))}
        {videos.map((v, i) => (
          <label key={`v${v.id}`} className={`hp-split-tile${pick[`v${v.id}`] ? ' on' : ''}`}>
            <input type="checkbox" checked={!!pick[`v${v.id}`]} onChange={() => toggle(`v${v.id}`)} />
            <video src={v.src} preload="metadata" muted playsInline />
            <span>Video {i + 1}</span>
          </label>
        ))}
      </div>
      <label className="field">
        <span>Địa chỉ công trình tách ra</span>
        <input className="input" value={addr} onChange={(e) => setAddr(e.target.value)} placeholder="VD: 6 Sơn Thủy 3, P. Ngũ Hành Sơn, Đà Nẵng" maxLength={500} />
      </label>
      <div className="row-wrap">
        <Button type="submit" loading={busy === 'split'} disabled={!photos.length || all || !addr.trim()}>
          Tách {photos.length ? `${photos.length} ảnh${vids.length ? `, ${vids.length} video` : ''}` : ''} ra hồ sơ mới
        </Button>
        {all && <small className="warn-text">Không tách hết ảnh: cả hồ sơ là công trình khác thì sửa địa chỉ.</small>}
      </div>
      <Lightbox ps={ps} at={at} setAt={setAt} />
    </form>
  )
}

function Mosaic({ ps, videos, masked }) {
  const [all, setAll] = useState(false)
  const [at, setAt] = useState(null) // ảnh đang xem lớn
  const [ratio, setRatio] = useState({}) // khoá ô -> rộng/cao
  const [play, setPlay] = useState(null) // id video đang phát
  const items = [
    ...ps.slice(0, 1).map((p) => ({ key: `p${p.id}`, p, k: 0 })),
    ...videos.map((v, i) => ({ key: `v${v.id}`, v, i })),
    ...ps.slice(1).map((p, i) => ({ key: `p${p.id}`, p, k: i + 1 })),
  ]
  const measure = (key, w, h) => w && h && setRatio((r) => (r[key] ? r : { ...r, [key]: Math.min(2, Math.max(0.5, w / h)) }))
  const shown = all ? items : items.slice(0, ROW * SHOW_ROWS)
  const rows = []
  for (let i = 0; i < shown.length; i += ROW) rows.push(shown.slice(i, i + ROW))
  const what = videos.length ? `${ps.length} ảnh và video` : `${ps.length} ảnh`
  return (
    <div className="hp-mosaic">
      {rows.map((row, r) => (
        <div key={r} className="hp-mrow">
          {row.map((it) => {
            const rt = ratio[it.key] || (it.v ? 9 / 16 : 4 / 3)
            const bad = it.p?.problems?.length > 0
            return (
              <div key={it.key} className="hp-tile" style={{ flex: `${rt} 1 0`, aspectRatio: rt }}>
                {it.p ? (
                  <button type="button" onClick={() => setAt(it.k)} aria-label={`Xem ảnh ${it.k + 1}`}>
                    <AuthImage path={`/photos/${it.p.id}${masked ? '?masked=1' : ''}`} alt={`Ảnh ${it.k + 1}`} onLoad={(e) => measure(it.key, e.target.naturalWidth, e.target.naturalHeight)} />
                  </button>
                ) : (
                  <video
                    src={it.v.src}
                    preload="metadata"
                    playsInline
                    controls={play === it.v.id}
                    autoPlay={play === it.v.id}
                    onLoadedMetadata={(e) => measure(it.key, e.target.videoWidth, e.target.videoHeight)}
                    onClick={() => play !== it.v.id && setPlay(it.v.id)}
                  />
                )}
                {it.p && it.k === 0 ? (
                  <span className={`hp-first ${bad ? 'bad' : ''}`}>
                    <i>{bad ? '✕' : '✓'}</i>Ảnh 1 · {bad ? 'Lỗi' : 'Đạt'}
                  </span>
                ) : it.p ? (
                  <span className={`hp-mark ${bad ? 'bad' : ''}`}>{bad ? '✕' : '✓'}</span>
                ) : (
                  play !== it.v.id && (
                    <span className="hp-vtag">
                      <svg width="10" height="10" viewBox="0 0 10 10" fill="#fff" aria-hidden="true">
                        <path d="M2 1l7 4-7 4z" />
                      </svg>
                      Video{videos.length > 1 ? ` ${it.i + 1}` : ''}
                    </span>
                  )
                )}
              </div>
            )
          })}
          {/* hàng cuối thiếu ô: chừa chỗ trống để ô không phình to */}
          {row.length < ROW && <div style={{ flex: `${(ROW - row.length) * (4 / 3)} 1 0` }} />}
        </div>
      ))}
      {items.length > ROW * SHOW_ROWS && (
        <button type="button" className="hp-all" onClick={() => setAll(!all)}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            {[2, 8, 14].flatMap((y) => [2, 8, 14].map((x) => <circle key={`${x}${y}`} cx={x} cy={y} r="1.5" />))}
          </svg>
          {all ? 'Thu gọn' : `Hiển thị tất cả ${what}`}
        </button>
      )}
      <Lightbox ps={ps} at={at} setAt={setAt} />
    </div>
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
  const [init] = useState(() => ({
    address: h.address || '',
    floors: h.floors ?? '',
    stage: h.stage || '',
    contractor_name: h.contractor_name || '',
    contractor_phone: h.contractor_phone || '',
  }))
  const [f, setF] = useState(init)
  // "Xem chi tiết": số tầng, giai đoạn (mở sẵn khi AI đọc khác giá trị đang lưu ở 2 ô này)
  const [more, setMore] = useState(() => h.ai_diff?.floors != null || h.ai_diff?.stage != null)
  const dirty = Object.keys(init).some((k) => String(f[k]) !== String(init[k]))
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
          <button type="button" className="hp-more" aria-expanded={more} onClick={() => setMore(!more)}>
            <span>{more ? 'Thu gọn' : 'Xem chi tiết'}</span>
            <svg width="12" height="12" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="4" aria-hidden="true">
              <path d={more ? 'M4 20l12-12 12 12' : 'M12 4l12 12-12 12'} />
            </svg>
          </button>
        </div>
        {more && (
          <div className="hp-cell-row">
            <label className="hp-cell">
              <span className="cap">Số tầng {ai('floors')}</span>
              <input value={f.floors} onChange={set('floors')} readOnly={!editable} inputMode="numeric" placeholder="—" />
              {diff('floors')}
            </label>
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
        )}
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
        {editable && (dirty || busy === 'info') && (
          <>
            <button type="button" className="link-btn" onClick={() => setF(init)}>
              Huỷ
            </button>
            <Button type="submit" variant="secondary" loading={busy === 'info'}>
              Lưu thông tin
            </Button>
          </>
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
