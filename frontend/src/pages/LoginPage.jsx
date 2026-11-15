import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import useAuth from '@/hooks/useAuth'
import { Button, Input } from '@/components/ui'
import { validateLoginForm, validateRegisterForm } from '@/utils/validators'

const EMPTY_FORM = { username: '', email: '', password: '' }

export default function LoginPage() {
  const { isAuthenticated, isLoading, error, login, register, clearError } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [mode, setMode] = useState('login') // 'login' | 'register'
  const [form, setForm] = useState(EMPTY_FORM)
  const [fieldErrors, setFieldErrors] = useState({})

  const isRegister = mode === 'register'
  const redirectTo = location.state?.from?.pathname || '/'

  if (isAuthenticated) return <Navigate to={redirectTo} replace />

  const handleChange = (e) => {
    const { name, value } = e.target
    setForm((prev) => ({ ...prev, [name]: value }))
    setFieldErrors((prev) => ({ ...prev, [name]: '' }))
  }

  const toggleMode = () => {
    setMode(isRegister ? 'login' : 'register')
    setFieldErrors({})
    clearError()
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    const errors = isRegister ? validateRegisterForm(form) : validateLoginForm(form)
    setFieldErrors(errors)
    if (Object.keys(errors).length) return

    try {
      if (isRegister) {
        await register({
          username: form.username.trim(),
          email: form.email.trim(),
          password: form.password,
        })
      } else {
        await login({ email: form.email.trim(), password: form.password })
      }
      navigate(redirectTo, { replace: true })
    } catch {
      // error message is stored in redux (auth.error)
    }
  }

  return (
    <section className="card auth-card">
      <h1>{isRegister ? 'Đăng ký' : 'Đăng nhập'}</h1>

      <form className="stack" onSubmit={handleSubmit} noValidate>
        {isRegister && (
          <Input
            label="Tên người dùng"
            name="username"
            autoComplete="username"
            value={form.username}
            onChange={handleChange}
            error={fieldErrors.username}
          />
        )}
        <Input
          label="Email"
          name="email"
          type="email"
          autoComplete="email"
          value={form.email}
          onChange={handleChange}
          error={fieldErrors.email}
        />
        <Input
          label="Mật khẩu"
          name="password"
          type="password"
          autoComplete={isRegister ? 'new-password' : 'current-password'}
          value={form.password}
          onChange={handleChange}
          error={fieldErrors.password}
        />

        {error && <p className="field-error">{error}</p>}

        <Button type="submit" loading={isLoading}>
          {isRegister ? 'Tạo tài khoản' : 'Đăng nhập'}
        </Button>
      </form>

      <p className="muted auth-switch">
        {isRegister ? 'Đã có tài khoản?' : 'Chưa có tài khoản?'}{' '}
        <button type="button" className="link-button" onClick={toggleMode}>
          {isRegister ? 'Đăng nhập' : 'Đăng ký'}
        </button>
      </p>
    </section>
  )
}
