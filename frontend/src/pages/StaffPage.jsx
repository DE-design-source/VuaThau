// Nhân viên (/staff, chỉ admin): danh sách admin, kế toán; thêm người, khoá, đặt lại mật khẩu.
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useAuth from '@/hooks/useAuth'
import adminApi from '@/api/adminApi'
import userApi from '@/api/userApi'
import { Button, PageHead, PageState, Pill } from '@/components/ui'
import { ROLE_LABEL } from '@/utils/format'

const EMPTY = { login: '', name: '', role: 'ketoan', password: '' }

export default function StaffPage() {
  const { data: d, err, reload } = usePage(() => adminApi.staff())
  const [act, busy] = useAct(reload)
  const { user } = useAuth()
  const [f, setF] = useState(EMPTY)
  const [pwFor, setPwFor] = useState({})
  if (!d) return <PageState err={err} retry={reload} />
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value })

  return (
    <section className="stack">
      <PageHead title="Nhân viên" sub="Tài khoản admin, kế toán và sale đăng nhập web này. Sale chỉ xem bản đồ, dự án, nhà thầu (không thấy CTV, không thấy tiền)." />
      <div className="card">
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Họ tên</th>
                <th>Đăng nhập</th>
                <th>Vai trò</th>
                <th>Mật khẩu mới</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(d.rows || []).map((r) => (
                <tr key={r.id}>
                  <td>
                    <strong>{r.name}</strong> {r.locked ? <Pill tone="bad">Đã khoá</Pill> : null}
                  </td>
                  <td>{r.username}</td>
                  <td>{ROLE_LABEL[r.role] || r.role}</td>
                  <td>
                    <div className="row-wrap">
                      <input
                        className="input"
                        placeholder="từ 6 ký tự"
                        value={pwFor[r.id] || ''}
                        onChange={(e) => setPwFor({ ...pwFor, [r.id]: e.target.value })}
                      />
                      <Button
                        variant="secondary"
                        disabled={(pwFor[r.id] || '').length < 6}
                        loading={busy === `pw${r.id}`}
                        onClick={() => act(`pw${r.id}`, () => userApi.setPassword(r.id, pwFor[r.id])).then((j) => j && setPwFor({ ...pwFor, [r.id]: '' }))}
                      >
                        Đặt
                      </Button>
                    </div>
                  </td>
                  <td>
                    {r.id !== user?.id && (
                      <Button
                        variant="ghost"
                        loading={busy === `lock${r.id}`}
                        onClick={() =>
                          window.confirm(`${r.locked ? 'Mở khoá' : 'Khoá'} ${r.name}?`) &&
                          act(`lock${r.id}`, () => userApi.lock(r.id, !r.locked))
                        }
                      >
                        {r.locked ? 'Mở khoá' : 'Khoá'}
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <form
        className="card stack"
        onSubmit={(e) => {
          e.preventDefault()
          act('add', () => adminApi.addStaff(f)).then((j) => j && setF(EMPTY))
        }}
      >
        <h2>Thêm nhân viên</h2>
        <div className="grid-2 tight">
          <label className="field">
            <span>Tên đăng nhập (a-z, 0-9, _) hoặc SĐT</span>
            <input className="input" value={f.login} onChange={set('login')} autoComplete="off" />
          </label>
          <label className="field">
            <span>Họ tên</span>
            <input className="input" value={f.name} onChange={set('name')} />
          </label>
          <label className="field">
            <span>Vai trò</span>
            <select className="input" value={f.role} onChange={set('role')}>
              <option value="ketoan">Kế toán</option>
              <option value="admin">Admin</option>
              <option value="sale">Sale (chỉ xem bản đồ, dự án, nhà thầu)</option>
            </select>
          </label>
          <label className="field">
            <span>Mật khẩu (từ 8 ký tự)</span>
            <input className="input" type="text" value={f.password} onChange={set('password')} autoComplete="new-password" />
          </label>
        </div>
        <Button type="submit" loading={busy === 'add'} disabled={!f.login || !f.name || f.password.length < 8}>
          Thêm nhân viên
        </Button>
      </form>
    </section>
  )
}
