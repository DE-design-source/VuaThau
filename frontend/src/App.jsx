import { useEffect } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import MainLayout from '@/components/layout/MainLayout'
import { PageState } from '@/components/ui'
import useAuth from '@/hooks/useAuth'
import { STAFF_HOME } from '@/redux/slices/authSlice'
import LoginPage from '@/pages/LoginPage'
import NotFoundPage from '@/pages/NotFoundPage'
import QueuePage from '@/pages/QueuePage'
import HousePage from '@/pages/HousePage'
import OverviewPage from '@/pages/OverviewPage'
import CtvListPage from '@/pages/CtvListPage'
import CtvPage from '@/pages/CtvPage'
import StaffPage from '@/pages/StaffPage'
import SettingsPage from '@/pages/SettingsPage'
import AccountingPage from '@/pages/AccountingPage'
import PayPage from '@/pages/PayPage'
import PaymentsPage from '@/pages/PaymentsPage'
import BatchPage from '@/pages/BatchPage'
import ReportPage from '@/pages/ReportPage'
import AccountPage from '@/pages/AccountPage'

/** Chỉ cho vào khi đã đăng nhập, và đúng vai trò nếu có roles. */
function Guard({ roles, children }) {
  const { isAuthenticated, user, error, fetchMe } = useAuth()
  const location = useLocation()
  if (!isAuthenticated) return <Navigate to="/login" replace state={{ from: location }} />
  if (!user) return <PageState err={error || ''} retry={() => fetchMe().catch(() => {})} />
  if (roles && !roles.includes(user.role)) return <Navigate to={STAFF_HOME[user.role] || '/me'} replace />
  return children
}

function Home() {
  const { user } = useAuth()
  return <Navigate to={STAFF_HOME[user?.role] || '/me'} replace />
}

const STAFF = ['admin', 'ketoan']

export default function App() {
  const { token, user, fetchMe } = useAuth()

  // Mở lại trang: đã có token thì lấy lại thông tin người dùng (vai trò).
  useEffect(() => {
    if (token && !user) fetchMe().catch(() => {})
  }, [token, user, fetchMe])

  const g = (el, roles = STAFF) => <Guard roles={roles}>{el}</Guard>

  return (
    <Routes>
      <Route element={<MainLayout />}>
        <Route index element={g(<Home />)} />
        <Route path="login" element={<LoginPage />} />
        <Route path="queue" element={g(<QueuePage />, ['admin'])} />
        <Route path="houses/:id" element={g(<HousePage />)} />
        <Route path="houses" element={g(<OverviewPage />)} />
        <Route path="ctvs" element={g(<CtvListPage />)} />
        <Route path="ctvs/:id" element={g(<CtvPage />)} />
        <Route path="staff" element={g(<StaffPage />, ['admin'])} />
        <Route path="settings" element={g(<SettingsPage />, ['admin'])} />
        <Route path="accounting" element={g(<AccountingPage />, ['ketoan'])} />
        <Route path="payouts/:ctvId" element={g(<PayPage />, ['ketoan'])} />
        <Route path="batches" element={g(<PaymentsPage />)} />
        <Route path="batches/:id" element={g(<BatchPage />)} />
        <Route path="reports" element={g(<ReportPage />)} />
        <Route path="me" element={g(<AccountPage />)} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
