import { Link, NavLink, useNavigate } from 'react-router-dom'
import logo from '@/assets/logo.svg'
import navLinks from '@/data/navLinks'
import useAuth from '@/hooks/useAuth'
import useTheme from '@/hooks/useTheme'
import { Button } from '@/components/ui'

export default function Header() {
  const { isAuthenticated, user, logout } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const navigate = useNavigate()

  const links = navLinks.filter(
    (link) => (!link.auth || isAuthenticated) && (!link.guest || !isAuthenticated),
  )

  const handleLogout = () => {
    logout()
    navigate('/login', { replace: true })
  }

  return (
    <header className="header">
      <div className="container header-inner">
        <Link to="/" className="brand">
          <img src={logo} alt="" width="28" height="28" />
          <span>Vuathau</span>
        </Link>

        <nav className="nav">
          {links.map((link) => (
            <NavLink key={link.to} to={link.to} end={link.end} className="nav-link">
              {link.label}
            </NavLink>
          ))}
        </nav>

        <div className="header-actions">
          <Button
            variant="ghost"
            onClick={toggleTheme}
            aria-label="Đổi giao diện sáng/tối"
            title="Đổi giao diện sáng/tối"
          >
            {theme === 'dark' ? '☀' : '☾'}
          </Button>
          {isAuthenticated ? (
            <>
              {user && <span className="muted">{user.username}</span>}
              <Button variant="secondary" onClick={handleLogout}>
                Đăng xuất
              </Button>
            </>
          ) : (
            <Button onClick={() => navigate('/login')}>Đăng nhập</Button>
          )}
        </div>
      </div>
    </header>
  )
}
