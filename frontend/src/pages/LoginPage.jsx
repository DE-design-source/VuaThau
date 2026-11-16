import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import useAuth from '@/hooks/useAuth'
import { Button, Input } from '@/components/ui'
import { Mark } from '@/components/layout/Header'
import { STAFF_HOME } from '@/redux/slices/authSlice'

export default function LoginPage() {
  const { isAuthenticated, isLoading, error, login, clearError } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [form, setForm] = useState({ login: '', password: '' })

  if (isAuthenticated) return <Navigate to={location.state?.from?.pathname || '/'} replace />

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
          <span className="eyebrow">Hệ thống nghiệm thu công trình</span>
          <h2>Mỗi công trình một hồ sơ. AI đọc ảnh, admin duyệt, kế toán trả tiền.</h2>
          <dl className="login-stats">
            <div>
              <dt>Đọc ảnh Timemark</dt>
              <dd className="mono">AI</dd>
            </div>
            <div>
              <dt>Chặn chụp trùng</dt>
              <dd className="mono">AI + GPS</dd>
            </div>
            <div>
              <dt>Bản đồ công trình</dt>
              <dd className="mono">Toàn quốc</dd>
            </div>
          </dl>
        </div>
        <small className="login-foot">App CTV · Messenger · Web quản lý</small>
      </aside>

      <section className="login-form">
        <div className="login-box">
          <div className="login-head">
            <h1>Đăng nhập</h1>
            <p className="muted">Dành cho admin, kế toán và sale.</p>
          </div>
          <form className="stack" onSubmit={submit} noValidate>
            <Input label="Tên đăng nhập hoặc số điện thoại" name="login" autoComplete="username" value={form.login} onChange={change} autoFocus />
            <Input label="Mật khẩu" name="password" type="password" autoComplete="current-password" value={form.password} onChange={change} />
            {error && <p className="field-error">{error}</p>}
            <Button type="submit" loading={isLoading} disabled={!form.login.trim() || !form.password}>
              Đăng nhập
            </Button>
          </form>
          <p className="muted small">Cộng tác viên đăng ký, nộp hồ sơ và xem thu nhập trên app Vua Thầu Map (Android, iPhone).</p>
        </div>
      </section>
    </div>
  )
}
