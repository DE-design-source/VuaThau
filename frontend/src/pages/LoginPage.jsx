import { useEffect, useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import useAuth from '@/hooks/useAuth'
import { Button, Icon, Input } from '@/components/ui'
import { Mark } from '@/components/layout/Header'
import { STAFF_HOME } from '@/redux/slices/authSlice'
import authApi from '@/api/authApi'
import configApi from '@/api/configApi'

// Đăng nhập (góp ý 24/11/2026): chọn vai trò trước.
//   Cộng tác viên / Nhà thầu: nhập SĐT -> kiểm tra trên máy chủ (đúng vai trò, đã có tài khoản) -> mở app web /app/
//     đăng nhập sẵn (link /app/?as=...&phone=...). CTV chưa có tài khoản đăng ký trong app; nhà thầu chưa có thì
//     mở form Đăng ký nhà thầu.
//   Quản lý (admin, kế toán, sale): tên đăng nhập + mật khẩu như cũ.
const ROLES = [
  { key: 'ctv', title: 'Cộng tác viên', sub: 'Chụp công trình, nhận tiền', icon: 'pin' },
  { key: 'nhathau', title: 'Nhà thầu', sub: 'Nhật ký thi công, hồ sơ năng lực', icon: 'houses' },
  { key: 'staff', title: 'Quản lý', sub: 'Admin, kế toán, sale', icon: 'staff' },
]

const onlyDigits = (s) => s.replace(/\D/g, '')

export default function LoginPage() {
  const { isAuthenticated, isLoading, error, login, clearError } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [role, setRole] = useState(() => {
    try {
      return localStorage.getItem('login-role') || ''
    } catch {
      return ''
    }
  })
  const [form, setForm] = useState({ login: '', password: '' })
  const [phone, setPhone] = useState('')
  const [phoneState, setPhoneState] = useState({ busy: false, err: '', switchTo: '', ntNew: false })
  const [ntForm, setNtForm] = useState('')

  useEffect(() => {
    configApi
      .get()
      .then((r) => setNtForm(r?.data?.nt_form || ''))
      .catch(() => {})
  }, [])

  if (isAuthenticated) return <Navigate to={location.state?.from?.pathname || '/'} replace />

  const pick = (k) => {
    setRole(k)
    setPhoneState({ busy: false, err: '', switchTo: '', ntNew: false })
    if (error) clearError()
    try {
      localStorage.setItem('login-role', k)
    } catch {
      /* trình duyệt chặn lưu: lần sau chọn lại */
    }
  }

  const change = (e) => {
    setForm((f) => ({ ...f, [e.target.name]: e.target.value }))
    if (error) clearError()
  }

  const submit = async (e) => {
    e.preventDefault()
    if (!form.login.trim() || !form.password) return
    try {
      const r = await login({ login: form.login.trim(), password: form.password })
      navigate(location.state?.from?.pathname || STAFF_HOME[r.user?.role] || '/', { replace: true })
    } catch {
      // câu báo nằm trong auth.error
    }
  }

  const goApp = (as, p) => window.location.assign(`/app/?as=${as}&phone=${encodeURIComponent(p)}`)

  const submitPhone = async (e, as = role) => {
    e?.preventDefault()
    const p = phone.trim()
    if (onlyDigits(p).length < 9) return
    setPhoneState({ busy: true, err: '', switchTo: '', ntNew: false })
    try {
      const r = await authApi.phone({ phone: p, as })
      if (r?.data?.registered || as === 'ctv') {
        // đã có tài khoản (hoặc CTV mới: app mở trang Đăng ký, điền sẵn SĐT)
        goApp(as, r?.data?.phone || p)
        return
      }
      setPhoneState({ busy: false, err: '', switchTo: '', ntNew: true })
    } catch (err) {
      const m = err.message || 'Có lỗi xảy ra.'
      setPhoneState({ busy: false, err: m, switchTo: m.includes('"Nhà thầu"') ? 'nhathau' : m.includes('"Cộng tác viên"') ? 'ctv' : '', ntNew: false })
    }
  }

  const cur = ROLES.find((r) => r.key === role)

  return (
    <div className="login">
      <aside className="login-art" aria-hidden="true">
        <div className="login-brand">
          <Mark size={56} />
          <span>
            <b>Vua Thầu Map</b>
          </span>
        </div>
        <div className="login-pitch">
          <span className="eyebrow">Bản đồ công trình toàn quốc</span>
          <h2>CTV chụp công trình nhận tiền. Nhà thầu ghi nhật ký thi công, gửi báo cáo cho chủ nhà.</h2>
          <dl className="login-stats">
            <div>
              <dt>Ảnh có giờ + GPS</dt>
              <dd className="mono">Xác minh</dd>
            </div>
            <div>
              <dt>Chặn chụp trùng</dt>
              <dd className="mono">GPS</dd>
            </div>
            <div>
              <dt>Bản đồ công trình</dt>
              <dd className="mono">Toàn quốc</dd>
            </div>
          </dl>
        </div>
        <small className="login-foot">App CTV · App nhà thầu · Web quản lý</small>
      </aside>

      <section className="login-form">
        <div className="login-box">
          <div className="login-head">
            <h1>Đăng nhập</h1>
            <p className="muted">Bạn là ai? Chọn 1 ô để tiếp tục.</p>
          </div>

          <div className="role-pick" role="radiogroup" aria-label="Vai trò">
            {ROLES.map((r) => (
              <button
                key={r.key}
                type="button"
                role="radio"
                aria-checked={role === r.key}
                className={`role-card${role === r.key ? ' on' : ''}`}
                onClick={() => pick(r.key)}
              >
                <span className="role-ico" aria-hidden="true">
                  <Icon name={r.icon} size={20} />
                </span>
                <b>{r.title}</b>
                <small>{r.sub}</small>
              </button>
            ))}
          </div>

          {role === 'staff' && (
            <form className="stack" onSubmit={submit} noValidate>
              <Input label="Tên đăng nhập hoặc số điện thoại" name="login" autoComplete="username" value={form.login} onChange={change} autoFocus />
              <Input label="Mật khẩu" name="password" type="password" autoComplete="current-password" value={form.password} onChange={change} />
              {error && <p className="field-error">{error}</p>}
              <Button type="submit" loading={isLoading} disabled={!form.login.trim() || !form.password}>
                Đăng nhập
              </Button>
            </form>
          )}

          {(role === 'ctv' || role === 'nhathau') && (
            <form className="stack" onSubmit={submitPhone} noValidate>
              <Input
                label="Số điện thoại"
                name="phone"
                type="tel"
                inputMode="tel"
                autoComplete="tel"
                placeholder="0901234567"
                value={phone}
                onChange={(e) => {
                  setPhone(e.target.value)
                  setPhoneState({ busy: false, err: '', switchTo: '', ntNew: false })
                }}
                autoFocus
              />
              <p className="muted small">
                {role === 'ctv'
                  ? 'Không cần mật khẩu. Chưa có tài khoản thì app chuyển sang đăng ký.'
                  : 'Không cần mật khẩu. Dùng số điện thoại Vua Thầu đã cấp cho công ty bạn.'}
              </p>
              {phoneState.err && <p className="field-error">{phoneState.err}</p>}
              {phoneState.switchTo && (
                <Button
                  type="button"
                  variant="secondary"
                  onClick={(e) => {
                    pick(phoneState.switchTo)
                    submitPhone(e, phoneState.switchTo)
                  }}
                >
                  Chuyển sang {phoneState.switchTo === 'nhathau' ? 'Nhà thầu' : 'Cộng tác viên'} và đăng nhập
                </Button>
              )}
              {phoneState.ntNew && (
                <div className="role-note">
                  <b>Số này chưa có tài khoản nhà thầu.</b>
                  <span>Đăng ký nhà thầu, Vua Thầu xác minh rồi cấp tài khoản theo số điện thoại này.</span>
                  {ntForm && (
                    <a className="btn btn-secondary" href={ntForm} target="_blank" rel="noreferrer">
                      Đăng ký nhà thầu
                    </a>
                  )}
                </div>
              )}
              <Button type="submit" loading={phoneState.busy} disabled={onlyDigits(phone).length < 9}>
                Tiếp tục
              </Button>
            </form>
          )}

          {!cur && <p className="muted small">CTV và nhà thầu vào bằng số điện thoại, dùng được trên điện thoại (app Android, iPhone) và trình duyệt.</p>}
        </div>
      </section>
    </div>
  )
}
