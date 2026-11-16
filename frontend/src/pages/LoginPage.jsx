import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import useAuth from '@/hooks/useAuth'
import { Button, Input } from '@/components/ui'
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
    <section className="card auth-card">
      <div className="auth-top">
        <img src="/favicon.svg" alt="" width="48" height="48" />
        <h1>Vua Thầu Nghiệm thu</h1>
        <p className="muted">Web quản lý cho admin và kế toán</p>
      </div>

      <form className="stack" onSubmit={submit} noValidate>
        <Input label="Tên đăng nhập hoặc số điện thoại" name="login" autoComplete="username" value={form.login} onChange={change} autoFocus />
        <Input label="Mật khẩu" name="password" type="password" autoComplete="current-password" value={form.password} onChange={change} />
        {error && <p className="field-error">{error}</p>}
        <Button type="submit" loading={isLoading} disabled={!form.login.trim() || !form.password}>
          Đăng nhập
        </Button>
      </form>

      <p className="muted auth-switch">Cộng tác viên: đăng ký, nộp hồ sơ và xem thu nhập trên app Vua Thầu (Android).</p>
    </section>
  )
}
