// 1 cộng tác viên (/ctvs/:id): tiền, hồ sơ, các lần nhận tiền, thưởng/trừ; admin khoá / đặt lại mật khẩu;
// khu vực, chức năng, ghi chú (admin sửa, kế toán chỉ xem); SĐT đăng nhập (admin nhập khi CTV Messenger chưa có SĐT);
// toàn bộ thông tin CTV bên Messenger (đồng bộ mỗi lượt đọc Messenger).
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useAuth from '@/hooks/useAuth'
import useCfg from '@/hooks/useCfg'
import ctvApi from '@/api/ctvApi'
import userApi from '@/api/userApi'
import { Button, HouseTable, Kpi, PageHead, PageState, Pill } from '@/components/ui'
import { money, vnDate, vnTime } from '@/utils/format'
import AdjustBox from './parts/AdjustBox'

export default function CtvPage() {
  const { id } = useParams()
  const { data: d, err, reload } = usePage(() => ctvApi.get(id), [id])
  const { role } = useAuth()
  const [act, busy] = useAct(reload)
  if (!d) return <PageState err={err} retry={reload} />
  const { user: ctv, money: m = {}, bank = {} } = d
  const admin = role === 'admin'
  const deleted = ctv.locked === 2

  return (
    <section className="stack">
      <PageHead title={ctv.name} sub={`${ctv.username}${ctv.email ? ` · Google: ${ctv.email}` : ''} · tham gia ${vnDate(ctv.created_at)}`}>
        {ctv.locked === 1 && <Pill tone="bad">Đã khoá</Pill>}
        {deleted && <Pill>CTV đã tự xoá tài khoản</Pill>}
        <Link className="btn btn-secondary" to={`/houses?ctv=${ctv.id}`}>
          Lọc hồ sơ của CTV này
        </Link>
        {role === 'ketoan' && (
          <Link className="btn btn-primary" to={`/payouts/${ctv.id}`}>
            Thanh toán
          </Link>
        )}
      </PageHead>

      <div className="kpis">
        <Kpi label="Đã nhận" value={money(m.paid)} tone="done" />
        <Kpi label="Chờ thanh toán" value={money(m.owed)} sub={`${m.approved} hồ sơ đã duyệt`} tone="ok" />
        <Kpi label="Đang chờ duyệt" value={money(m.reviewing)} tone="wait" />
      </div>

      <Profile key={JSON.stringify(d.profile)} id={ctv.id} p={d.profile || {}} editable={admin && !deleted} act={act} busy={busy} />

      {d.inbox && <InboxInfo info={d.inbox} at={d.inbox_at} />}

      <div className="grid-2">
        <div className="card stack">
          <h2>Tài khoản nhận tiền</h2>
          <p>{bank.owner ? `${bank.name} · ${bank.no} · ${bank.owner}` : ctv.bank || <span className="muted">Chưa có</span>}</p>
          {admin && !deleted && (
            <>
              <div className="row-wrap">
                <Button
                  variant="secondary"
                  className={ctv.locked ? '' : 'btn-danger'}
                  loading={busy === 'lock'}
                  onClick={() =>
                    window.confirm(ctv.locked ? `Mở khoá ${ctv.name}?` : `Khoá ${ctv.name}? CTV sẽ bị đăng xuất khỏi app.`) &&
                    act('lock', () => userApi.lock(ctv.id, !ctv.locked))
                  }
                >
                  {ctv.locked ? 'Mở khoá tài khoản' : 'Khoá tài khoản'}
                </Button>
              </div>
            </>
          )}
        </div>
        {admin && !deleted && <LoginBox key={ctv.username} ctv={ctv} loginPhone={d.login_phone} inboxPhone={d.inbox?.phone} act={act} busy={busy} />}
        <AdjustBox ctvId={ctv.id} adjs={d.adjustments || []} act={act} busy={busy} />
      </div>

      <div className="card stack">
        <h2>Các lần nhận tiền</h2>
        {d.pays?.length ? (
          <ul className="list">
            {d.pays.map((p) => (
              <li key={p.id} className="row-between">
                <Link to={`/batches/${p.id}`}>
                  Phiếu chi #{p.id} · {vnDate(p.created_at)} · {p.house_count} hồ sơ{p.ref ? ` · ${p.ref}` : ''}
                </Link>
                <strong>{money(p.total)}</strong>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">Chưa nhận lần nào.</p>
        )}
      </div>

      <div className="card stack">
        <h2>Hồ sơ ({d.rows?.length || 0})</h2>
        <HouseTable rows={d.rows || []} showCtv={false} />
      </div>
    </section>
  )
}

/** SĐT = tên đăng nhập app; mật khẩu admin đặt rồi gửi CTV. Chưa có SĐT: lấy SĐT bên Messenger (nếu có), không
 *  có thì ô để trống, tô vàng để admin nhập. */
function LoginBox({ ctv, loginPhone, inboxPhone, act, busy }) {
  const init = loginPhone ? ctv.username : phoneOf(inboxPhone)
  const [phone, setPhone] = useState(init)
  const [pw, setPw] = useState('')
  const need = !loginPhone
  const digits = phone.replace(/\D/g, '')
  return (
    <div className="card stack">
      <h2>Đăng nhập app</h2>
      <form
        className="stack tight"
        onSubmit={(e) => {
          e.preventDefault()
          act('login', () => ctvApi.setLogin(ctv.id, phone))
        }}
      >
        <label className="field">
          <span>Tài khoản (SĐT)</span>
          <div className="row-wrap">
            <input
              className={`input grow${need && !digits ? ' input-need' : ''}`}
              inputMode="tel"
              placeholder="Nhập SĐT của CTV"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
            />
            <Button type="submit" variant="secondary" disabled={digits.length < 10 || (loginPhone && phone === ctv.username)} loading={busy === 'login'}>
              Lưu SĐT
            </Button>
          </div>
          {need && (
            <small className={digits ? 'muted' : 'warn-text'}>
              {digits ? (phone === init && inboxPhone ? 'SĐT lấy từ Messenger, bấm Lưu SĐT để dùng làm tài khoản.' : 'Bấm Lưu SĐT để dùng làm tài khoản.') : `Chưa có SĐT (đang là ${ctv.username}), Messenger cũng chưa có: nhập SĐT để CTV đăng nhập app.`}
            </small>
          )}
        </label>
      </form>
      <form
        className="row-wrap"
        onSubmit={(e) => {
          e.preventDefault()
          act('pw', () => userApi.setPassword(ctv.id, pw)).then((j) => j && setPw(''))
        }}
      >
        <input className="input grow" type="text" placeholder="Mật khẩu mới (từ 6 ký tự)" value={pw} onChange={(e) => setPw(e.target.value)} />
        <Button type="submit" variant="secondary" disabled={pw.length < 6} loading={busy === 'pw'}>
          Đặt lại mật khẩu
        </Button>
      </form>
    </div>
  )
}

/** "+84 707 ..." -> "0707..." */
function phoneOf(s) {
  const d = (s || '').replace(/\D/g, '')
  return d.startsWith('84') && d.length >= 11 ? `0${d.slice(2)}` : d
}

const INBOX_LABELS = { need: 'Nhu cầu', khu_vuc: 'Khu vực', loai_hinh_doi_tac: 'Loại hình đối tác', thoi_diem_thi_cong: 'Thời điểm thi công', so_tai_khoan: 'Số tài khoản' }

/** Toàn bộ thông tin CTV bên Messenger (khung "Thông tin khách" của dezon-inbox), chỉ xem. */
function InboxInfo({ info, at }) {
  const rows = [
    ['Tên Messenger', info.name],
    ['SĐT', info.phone],
    ['Email', info.email],
    ...Object.entries(info.fields || {}).map(([k, v]) => [info.labels?.[k] || INBOX_LABELS[k] || k, v]),
    ['Nhãn', (info.tags || []).join(', ')],
    ['Ghi chú', info.notes],
  ]
  return (
    <div className="card stack">
      <div className="row-between">
        <h2>Thông tin trên Messenger</h2>
        {at && <small className="muted">Đồng bộ {vnTime(at)}</small>}
      </div>
      <dl className="ctv-inbox">
        {rows.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd className={v ? '' : 'muted'}>{v || 'Chưa có'}</dd>
          </div>
        ))}
      </dl>
      <small className="muted">Ô Khu vực, Ghi chú, Tài khoản nhận tiền của web còn trống thì tự điền từ đây; ô đã nhập không bị ghi đè.</small>
    </div>
  )
}

/** Khu vực (để trống = tự tính theo tỉnh CTV chụp nhiều nhất), chức năng, ghi chú nội bộ. */
function Profile({ id, p, editable, act, busy }) {
  const cfg = useCfg()
  const provinces = [...(cfg?.provinces || [])].sort((a, b) => a.name.localeCompare(b.name, 'vi'))
  const name = Object.fromEntries(provinces.map((x) => [x.code, x.name]))
  const kinds = cfg?.ctv_kinds || ['CTV']
  const init = { region: p.region || '', kind: p.kind || 'CTV', note: p.note || '' }
  const [f, setF] = useState(init)
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value })
  const dirty = Object.keys(init).some((k) => f[k] !== init[k])
  const auto = p.region_auto ? `Tự tính: ${name[p.region_auto] || p.region_auto}` : 'Tự tính (chưa có công trình)'
  return (
    <form
      className="card stack ctv-profile"
      onSubmit={(e) => {
        e.preventDefault()
        act('profile', () => ctvApi.setProfile(id, f))
      }}
    >
      <h2>Khu vực, chức năng, ghi chú</h2>
      <div className="grid-2 tight">
        <label className="field">
          <span>Khu vực</span>
          {editable ? (
            <select className="input" value={f.region} onChange={set('region')}>
              <option value="">{auto}</option>
              {provinces.map((x) => (
                <option key={x.code} value={x.code}>
                  {x.name}
                </option>
              ))}
            </select>
          ) : (
            <input className="input" readOnly value={p.region ? name[p.region] || p.region : auto} />
          )}
          <small className="muted">Để "Tự tính" thì lấy tỉnh CTV chụp nhiều công trình nhất.</small>
        </label>
        <label className="field">
          <span>Chức năng</span>
          {editable ? (
            <select className="input" value={f.kind} onChange={set('kind')}>
              {[...new Set([...kinds, f.kind])].map((k) => (
                <option key={k}>{k}</option>
              ))}
            </select>
          ) : (
            <input className="input" readOnly value={p.kind || 'CTV'} />
          )}
          {editable && <small className="muted">Sửa danh sách chức năng ở Cấu hình.</small>}
        </label>
      </div>
      <label className="field">
        <span>Ghi chú khác</span>
        <textarea className="input" rows={3} value={f.note} onChange={set('note')} readOnly={!editable} maxLength={1000} placeholder={editable ? 'Chỉ nhân viên thấy, CTV không thấy' : '—'} />
      </label>
      {editable && (
        <div className="row-wrap">
          <Button type="submit" loading={busy === 'profile'} disabled={!dirty}>
            Lưu
          </Button>
          {dirty && (
            <Button variant="ghost" type="button" onClick={() => setF(init)}>
              Huỷ
            </Button>
          )}
        </div>
      )}
    </form>
  )
}
