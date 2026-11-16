// Thanh bên điều hướng (desktop cố định bên trái, điện thoại mở bằng nút menu) + thanh trên mỏng.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import navLinks from '@/data/navLinks'
import useAuth from '@/hooks/useAuth'
import useTheme from '@/hooks/useTheme'
import Icon from '@/components/ui/Icon'
import { ROLE_LABEL, initials } from '@/utils/format'

export function Mark({ size = 28 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="7" fill="#F5B301" />
      <path d="M8.5 16.5l5 5 10-11" fill="none" stroke="#0B1F3A" strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export default function Header() {
  const { user, role, logout } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)

  const links = navLinks.filter((l) => role && l.roles.includes(role))
  const groups = [...new Set(links.map((l) => l.group))]
  const current = links.find((l) => pathname.startsWith(l.to))
  const todo = user?.todo || 0

  const handleLogout = () => {
    if (!window.confirm('Đăng xuất khỏi web quản lý?')) return
    logout()
    navigate('/login', { replace: true })
  }

  return (
    <>
      <aside className={`side ${open ? 'side-open' : ''}`} aria-label="Điều hướng">
        <Link to="/" className="side-brand" onClick={() => setOpen(false)}>
          <Mark />
          <span>
            <b>Vua Thầu</b>
            <small>Nghiệm thu</small>
          </span>
        </Link>
        <nav className="side-nav" onClick={() => setOpen(false)}>
          {groups.map((g) => (
            <div key={g} className="side-group">
              <span className="side-label">{g}</span>
              {links
                .filter((l) => l.group === g)
                .map((l) => (
                  <NavLink key={l.to} to={l.to} className="side-link">
                    <Icon name={l.icon} />
                    <span>{l.label}</span>
                    {l.badge && todo > 0 && <em className="side-count">{todo}</em>}
                  </NavLink>
                ))}
            </div>
          ))}
        </nav>
        {user && (
          <div className="side-user">
            <Link to="/me" className="side-me" onClick={() => setOpen(false)} title="Tài khoản">
              <span className="avatar">{initials(user.name)}</span>
              <span>
                <b>{user.name}</b>
                <small>{ROLE_LABEL[user.role]}</small>
              </span>
            </Link>
            <button type="button" className="side-icon" onClick={handleLogout} title="Đăng xuất" aria-label="Đăng xuất">
              <Icon name="logout" />
            </button>
          </div>
        )}
      </aside>
      {open && <div className="side-scrim" onClick={() => setOpen(false)} />}

      <header className="topbar">
        <button type="button" className="side-icon topbar-menu" aria-expanded={open} aria-label="Mở menu" onClick={() => setOpen(!open)}>
          <Icon name="menu" />
        </button>
        <div className="crumbs">
          <span>Vua Thầu</span>
          <Icon name="right" size={14} />
          <b>{current?.label || (pathname.startsWith('/me') ? 'Tài khoản' : 'Tổng quan')}</b>
        </div>
        <span className="spacer" />
        <span className="live">
          <i /> Đang hoạt động
        </span>
        <button type="button" className="side-icon" onClick={toggleTheme} aria-label="Đổi giao diện sáng/tối" title="Đổi giao diện sáng/tối">
          <Icon name={theme === 'dark' ? 'sun' : 'moon'} />
        </button>
      </header>
    </>
  )
}
