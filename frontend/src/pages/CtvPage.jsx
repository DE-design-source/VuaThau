// 1 cộng tác viên (/ctvs/:id): tiền, hồ sơ, các lần nhận tiền, thưởng/trừ; admin khoá / đặt lại mật khẩu;
// khu vực, chức năng, ghi chú (admin sửa, kế toán chỉ xem).
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useAuth from '@/hooks/useAuth'
import useCfg from '@/hooks/useCfg'
import ctvApi from '@/api/ctvApi'
import userApi from '@/api/userApi'
import { Button, HouseTable, Kpi, PageHead, PageState, Pill } from '@/components/ui'
import { money, vnDate } from '@/utils/format'
import AdjustBox from './parts/AdjustBox'

export default function CtvPage() {
  const { id } = useParams()
  const { data: d, err, reload } = usePage(() => ctvApi.get(id), [id])
  const { role } = useAuth()
  const [act, busy] = useAct(reload)
  const [pw, setPw] = useState('')
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
            </>
          )}
        </div>
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
