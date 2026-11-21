// Danh sách cộng tác viên (/ctvs): tìm theo tên, SĐT, email, ghi chú; lọc theo khu vực, chức năng, tài khoản ngân hàng.
// Cột: SĐT, khu vực (admin chọn hoặc tỉnh chụp nhiều nhất), chức năng, số tài khoản, số hồ sơ, tiền, ghi chú.
// Admin: tạo tài khoản CTV (đăng nhập app bằng SĐT + mật khẩu admin đặt).
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useAuth from '@/hooks/useAuth'
import useCfg from '@/hooks/useCfg'
import ctvApi from '@/api/ctvApi'
import { Button, Empty, Icon, PageHead, PageState, Pager, Pill } from '@/components/ui'
import { bankParts, money, vnDate } from '@/utils/format'

const APP_URL = 'https://map.vuathau.com.vn/tai-app'
const EMPTY = { name: '', phone: '', password: '', bank_name: '', bank_no: '', bank_owner: '' }

// Mật khẩu 6 số, dễ đọc qua điện thoại / Zalo
function randomPin() {
  const a = new Uint32Array(1)
  crypto.getRandomValues(a)
  return String(a[0] % 1000000).padStart(6, '0')
}

function AddCtv({ reload, onClose }) {
  const [act, busy] = useAct(reload)
  const cfg = useCfg()
  const [f, setF] = useState(() => ({ ...EMPTY, password: randomPin() }))
  const [done, setDone] = useState(null) // { id, name, phone, password }
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value })
  const phoneOk = /^(0|\+84)\d{9,10}$/.test(f.phone.replace(/[\s.-]/g, ''))

  if (done) {
    const text = `Tài khoản app Vua Thầu Map của ${done.name}:\n- Tải app (Android): ${APP_URL}\n- Đăng nhập: SĐT ${done.phone}\n- Mật khẩu: ${done.password}\nĐăng nhập xong vào Tài khoản để thêm / kiểm tra tài khoản ngân hàng nhận tiền.`
    return (
      <div className="card stack">
        <h2>Đã tạo tài khoản CTV {done.name}</h2>
        <p className="muted">Gửi nội dung dưới cho CTV qua Zalo / tin nhắn:</p>
        <textarea className="input mono" rows={5} readOnly value={text} onFocus={(e) => e.target.select()} />
        <div className="row-wrap">
          <Button onClick={() => navigator.clipboard?.writeText(text)}>
            <Icon name="check" size={16} /> Chép nội dung
          </Button>
          <Link className="btn btn-secondary" to={`/ctvs/${done.id}`}>
            Mở trang CTV
          </Link>
          <Button
            variant="ghost"
            onClick={() => {
              setDone(null)
              setF({ ...EMPTY, password: randomPin() })
            }}
          >
            Tạo thêm CTV khác
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Đóng
          </Button>
        </div>
      </div>
    )
  }

  return (
    <form
      className="card stack"
      onSubmit={(e) => {
        e.preventDefault()
        act('add', () => ctvApi.add(f)).then(
          (j) => j?.user && setDone({ id: j.user.id, name: j.user.name, phone: j.user.username, password: f.password }),
        )
      }}
    >
      <div className="row-between">
        <h2>Tạo tài khoản CTV</h2>
        <Button variant="ghost" type="button" onClick={onClose} aria-label="Đóng">
          <Icon name="x" />
        </Button>
      </div>
      <p className="muted">CTV đăng nhập app bằng số điện thoại và mật khẩu này. Ngân hàng có thể để trống, CTV tự thêm trong app.</p>
      <div className="grid-2 tight">
        <label className="field">
          <span>Họ tên</span>
          <input className="input" value={f.name} onChange={set('name')} autoComplete="off" />
        </label>
        <label className="field">
          <span>Số điện thoại (dùng để đăng nhập)</span>
          <input className="input" inputMode="tel" value={f.phone} onChange={set('phone')} autoComplete="off" />
          {f.phone && !phoneOk && <small className="text-bad">SĐT 10-11 số, bắt đầu bằng 0 hoặc +84</small>}
        </label>
        <label className="field">
          <span>Mật khẩu (từ 6 ký tự)</span>
          <span className="row-wrap">
            <input className="input mono" value={f.password} onChange={set('password')} autoComplete="new-password" />
            <Button variant="ghost" type="button" onClick={() => setF({ ...f, password: randomPin() })}>
              Tạo mật khẩu khác
            </Button>
          </span>
        </label>
      </div>
      <details>
        <summary className="muted">Tài khoản ngân hàng nhận tiền (không bắt buộc)</summary>
        <div className="grid-2 tight" style={{ marginTop: 12 }}>
          <label className="field">
            <span>Ngân hàng</span>
            <select className="input" value={f.bank_name} onChange={set('bank_name')}>
              <option value="">—</option>
              {(cfg?.banks || []).map((b) => (
                <option key={b}>{b}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>Số tài khoản</span>
            <input className="input" inputMode="numeric" value={f.bank_no} onChange={set('bank_no')} />
          </label>
          <label className="field">
            <span>Chủ tài khoản</span>
            <input className="input" value={f.bank_owner} onChange={set('bank_owner')} />
          </label>
        </div>
      </details>
      <Button type="submit" loading={busy === 'add'} disabled={!f.name.trim() || !phoneOk || f.password.length < 6}>
        Tạo tài khoản
      </Button>
    </form>
  )
}

const BANK_OPTS = [
  { value: '', label: 'Tất cả' },
  { value: 'co', label: 'Đã có' },
  { value: 'chua', label: 'Chưa có' },
]
const PHONE_RE = /^\+?\d[\d\s.]{7,}$/

export default function CtvListPage() {
  const [sp, setSp] = useSearchParams()
  const cur = { q: sp.get('q') || '', region: sp.get('region') || '', kind: sp.get('kind') || '', bank: sp.get('bank') || '', status: sp.get('status') || '', page: sp.get('page') || '' }
  const { data: d, err, busy, reload } = usePage(() => ctvApi.list(cur), [cur.q, cur.region, cur.kind, cur.bank, cur.status, cur.page])
  const [q, setQ] = useState(cur.q)
  const { role } = useAuth()
  const cfg = useCfg()
  const [adding, setAdding] = useState(false)
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []
  const facets = d.facets || {}
  const provName = Object.fromEntries((cfg?.provinces || []).map((p) => [p.code, p.name]))
  // đổi bộ lọc: về trang 1
  const go = (patch) => {
    const next = { ...cur, page: '', ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([k, v]) => v && !(k === 'page' && Number(v) <= 1))))
  }
  const opts = (list = [], all = 'Tất cả') => [{ value: '', label: all }, ...list.map((x) => ({ value: x.value, label: `${x.label} (${x.count})` }))]
  const selects = [
    { key: 'region', label: 'Khu vực', options: opts(facets.regions, 'Mọi khu vực') },
    { key: 'kind', label: 'Chức năng', options: opts(facets.kinds, 'Mọi chức năng') },
    { key: 'bank', label: 'Tài khoản NH', options: BANK_OPTS },
    // Tích cực: từ 10 công trình trong 30 ngày; Đã đăng ký: còn lại (đăng ký qua Messenger / form tuyển, có SĐT)
    {
      key: 'status',
      label: 'Trạng thái',
      options: [
        { value: '', label: 'Tất cả' },
        { value: 'tich_cuc', label: 'Tích cực (từ 10 công trình / 30 ngày)' },
        { value: 'dang_ky', label: 'Đã đăng ký' },
      ],
    },
  ]
  const filtered = Boolean(cur.q || cur.region || cur.kind || cur.bank || cur.status)

  return (
    <section className="stack">
      <PageHead title="Cộng tác viên" sub={`${d.found} người${filtered ? ' theo bộ lọc' : ''}. CTV tự đăng ký trên app, hoặc admin tạo tài khoản ở đây.`}>
        {role === 'admin' && !adding && (
          <Button onClick={() => setAdding(true)}>
            <Icon name="plus" size={16} /> Tạo tài khoản CTV
          </Button>
        )}
      </PageHead>
      {adding && <AddCtv reload={reload} onClose={() => setAdding(false)} />}
      <form
        className="card filters ctv-filters"
        onSubmit={(e) => {
          e.preventDefault()
          go({ q })
        }}
      >
        <input className="input" placeholder="Tìm tên, SĐT, email, ghi chú" value={q} onChange={(e) => setQ(e.target.value)} />
        {selects.map((x) => (
          <label key={x.key} className="ctv-sel">
            <span>{x.label}</span>
            <select className="input" value={cur[x.key]} onChange={(e) => go({ [x.key]: e.target.value })} aria-label={x.label}>
              {x.options.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
        ))}
        <Button type="submit" loading={busy}>
          Tìm
        </Button>
        {filtered && (
          <Button
            variant="ghost"
            type="button"
            onClick={() => {
              setQ('')
              setSp({})
            }}
          >
            Bỏ lọc
          </Button>
        )}
      </form>
      <div className="card">
        {rows.length ? (
          <div className="table-wrap">
            <table className="table ctv-table">
              <thead>
                <tr>
                  <th>CTV</th>
                  <th>Số điện thoại</th>
                  <th>Trạng thái</th>
                  <th>Địa chỉ</th>
                  <th>Khu vực</th>
                  <th>Chức năng</th>
                  <th>Số tài khoản</th>
                  <th className="num">Hồ sơ</th>
                  <th className="num">Chờ duyệt</th>
                  <th className="num">Đã duyệt</th>
                  <th className="num">Đã trả</th>
                  <th className="num">Còn nợ</th>
                  <th>Nộp gần nhất</th>
                  <th>Ghi chú</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => {
                  const b = bankParts(r.bank)
                  return (
                    <tr key={r.id}>
                      <td>
                        <Link to={`/ctvs/${r.id}`}>
                          <strong className="ctv-name">{r.name}</strong>
                        </Link>{' '}
                        {r.locked === 1 && <Pill tone="bad">Đã khoá</Pill>}
                        {r.locked === 2 && <Pill>Đã xoá</Pill>}
                        {r.email && <small className="muted d-block">{r.email}</small>}
                      </td>
                      <td className="nowrap">{PHONE_RE.test(r.username) ? r.username : <span className="muted">{r.username}</span>}</td>
                      <td className="nowrap">
                        {r.active ? <Pill tone="ok">Tích cực</Pill> : <Pill>Đã đăng ký</Pill>}
                        <small className="muted d-block">{r.recent} hồ sơ / 30 ngày</small>
                      </td>
                      <td className="ctv-addr-col" title={r.address || ''}>
                        {r.address || <span className="muted">—</span>}
                      </td>
                      <td className="nowrap">
                        {r.region ? (
                          <span title={r.region_auto ? 'Tự tính: tỉnh CTV chụp nhiều công trình nhất' : 'Admin chọn'}>
                            {provName[r.region] || r.region}
                            {r.region_auto && <small className="muted d-block">tự tính</small>}
                          </span>
                        ) : (
                          <span className="muted">—</span>
                        )}
                      </td>
                      <td className="nowrap">{r.kind || '—'}</td>
                      <td className="nowrap">
                        {b.no ? (
                          <>
                            {b.no}
                            <small className="muted d-block">{b.name}</small>
                          </>
                        ) : r.bank ? (
                          <small>{r.bank}</small>
                        ) : (
                          <span className="muted">Chưa có</span>
                        )}
                      </td>
                      <td className="num">{r.houses}</td>
                      <td className="num">{r.waiting}</td>
                      <td className="num">{r.approved}</td>
                      <td className="num">{money(r.paid)}</td>
                      <td className="num">{money(r.owed)}</td>
                      <td className="muted nowrap">{vnDate(r.last_at) || '—'}</td>
                      <td className="ctv-note" title={r.note || ''}>
                        {r.note || <span className="muted">—</span>}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="Không tìm thấy cộng tác viên" text={filtered ? 'Thử bỏ bớt bộ lọc.' : undefined} />
        )}
        <Pager page={d.page} pages={d.pages} onGo={(p) => go({ page: String(p) })} />
      </div>
    </section>
  )
}
