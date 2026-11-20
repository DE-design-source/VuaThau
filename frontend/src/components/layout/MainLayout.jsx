import { Outlet, useLocation } from 'react-router-dom'
import { Toasts } from '@/components/ui'
import useAuth from '@/hooks/useAuth'
import Header from './Header'
import Footer from './Footer'

export default function MainLayout() {
  const { isAuthenticated } = useAuth()
  const { pathname } = useLocation()
  // Chưa đăng nhập: trang đăng nhập toàn màn hình, không thanh bên. Bản đồ, Xem quy hoạch: có thanh bên (chế độ khách).
  const open = pathname.startsWith('/ban-do') || pathname.startsWith('/quy-hoach')
  if (!isAuthenticated && !open) {
    return (
      <div className="auth-layout">
        <Outlet />
        <Toasts />
      </div>
    )
  }
  return (
    <div className="shell">
      <Header />
      <div className="shell-main">
        <main className="main">
          <Outlet />
        </main>
        <Footer />
      </div>
      <Toasts />
    </div>
  )
}
