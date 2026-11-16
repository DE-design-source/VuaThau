import { Outlet } from 'react-router-dom'
import { Toasts } from '@/components/ui'
import useAuth from '@/hooks/useAuth'
import Header from './Header'
import Footer from './Footer'

export default function MainLayout() {
  const { isAuthenticated } = useAuth()
  // Chưa đăng nhập: trang đăng nhập toàn màn hình, không thanh bên
  if (!isAuthenticated) {
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
