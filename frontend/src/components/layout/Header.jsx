import { useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import logo from '@/assets/logo.svg'
import navLinks from '@/data/navLinks'
import useAuth from '@/hooks/useAuth'
import useTheme from '@/hooks/useTheme'
import { Button } from '@/components/ui'
import { ROLE_LABEL } from '@/utils/format'

export default function Header() {
  const { isAuthenticated, user, role, logout } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)

  const links = navLinks.filter((l) => role && l.roles.includes(role))

  const handleLogout = () => {
    if (!window.confirm('Đăng xuất khỏi web quản lý?')) return
    logout()
    navigate('/login', { replace: true })
  }

  return (
    <header className="header">
      <div className="container header-inner">
        <Link to="/" className="brand">
          <img src={logo} alt="" width="30" height="30" />
          <span>
            Vua Thầu <small>Nghiệm thu</small>
          </span>
        </Link>

        {isAuthenticated && (
          <button type="button" className="btn btn-ghost menu-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
            ☰ Menu
          </button>
        )}

        <nav className={`nav ${open ? 'nav-open' : ''}`} onClick={() => setOpen(false)}>
          {links.map((l) => (
            <NavLink key={l.to} to={l.to} className="nav-link">
              {l.label}
            </NavLink>
          ))}
        </nav>

        <div className="header-actions">
          <Button variant="ghost" onClick={toggleTheme} aria-label="Đổi giao diện sáng/tối" title="Đổi giao diện sáng/tối">
            {theme === 'dark' ? '☀' : '☾'}
          </Button>
          {isAuthenticated && (
            <>
              {user && (
                <Link to="/toi" className="who" title="Tài khoản">
                  <strong>{user.name}</strong>
                  <small className="muted">{ROLE_LABEL[user.role]}</small>
                </Link>
              )}
              <Button variant="secondary" onClick={handleLogout}>
                Đăng xuất
              </Button>
            </>
          )}
        </div>
      </div>
    </header>
  )
}
