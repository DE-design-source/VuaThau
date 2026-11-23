import { lazy, Suspense, useEffect } from 'react'
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
import SaleProjectsPage from '@/pages/sale/SaleProjectsPage'
import SaleProjectPage from '@/pages/sale/SaleProjectPage'
import ContractorsPage from '@/pages/sale/ContractorsPage'
// Bản đồ (Leaflet) tải riêng khi mở trang, không làm nặng các trang khác
const MapPage = lazy(() => import('@/pages/MapPage'))
const PlanPage = lazy(() => import('@/pages/PlanPage'))
const PublicMapPage = lazy(() => import('@/pages/PublicMapPage'))
const PublicProjectPage = lazy(() => import('@/pages/PublicProjectPage'))
const ContractorListPage = lazy(() => import('@/pages/ContractorListPage'))
const ContractorPage = lazy(() => import('@/pages/ContractorPage'))
const SaleMapPage = lazy(() => import('@/pages/sale/SaleMapPage'))

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

/** Trang chủ: đã đăng nhập thì về trang theo vai trò, chưa thì xem Bản đồ công khai. */
function Index() {
  const { isAuthenticated } = useAuth()
  if (!isAuthenticated) return <Navigate to="/ban-do" replace />
  return (
    <Guard>
      <Home />
    </Guard>
  )
}

/** Bản đồ công khai: người đã đăng nhập về bản đồ theo vai trò (đủ thông tin hơn). */
function PublicMap() {
  const { isAuthenticated, user } = useAuth()
  if (isAuthenticated && user) return <Navigate to={user.role === 'sale' ? '/sale/map' : STAFF.includes(user.role) ? '/map' : '/me'} replace />
  return (
    <Suspense fallback={<PageState err="" />}>
      <PublicMapPage />
    </Suspense>
  )
}

const STAFF = ['admin', 'ketoan']
// Trang sale: không có CTV, không có tiền. Admin vào được để xem sale thấy gì.
const SALE = ['sale', 'admin']

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
        <Route index element={<Index />} />
        <Route path="ban-do" element={<PublicMap />} />
        <Route path="ban-do/:id" element={<Suspense fallback={<PageState err="" />}><PublicProjectPage /></Suspense>} />
        <Route path="login" element={<LoginPage />} />
        <Route path="queue" element={g(<QueuePage />, ['admin'])} />
        <Route path="houses/:id" element={g(<HousePage />)} />
        <Route path="houses" element={g(<OverviewPage />)} />
        <Route path="ctvs" element={g(<CtvListPage />)} />
        <Route path="ctvs/:id" element={g(<CtvPage />)} />
        <Route path="contractors" element={g(<Suspense fallback={<PageState err="" />}><ContractorListPage /></Suspense>)} />
        <Route path="contractors/:key" element={g(<Suspense fallback={<PageState err="" />}><ContractorPage /></Suspense>)} />
        <Route path="map" element={g(<Suspense fallback={<PageState err="" />}><MapPage /></Suspense>)} />
        <Route path="quy-hoach" element={<Suspense fallback={<PageState err="" />}><PlanPage /></Suspense>} />
        <Route path="sale/map" element={g(<Suspense fallback={<PageState err="" />}><SaleMapPage /></Suspense>, SALE)} />
        <Route path="sale/projects" element={g(<SaleProjectsPage />, SALE)} />
        <Route path="sale/projects/:id" element={g(<SaleProjectPage />, SALE)} />
        <Route path="sale/contractors" element={g(<ContractorsPage />, SALE)} />
        <Route path="staff" element={g(<StaffPage />, ['admin'])} />
        <Route path="settings" element={g(<SettingsPage />, ['admin'])} />
        <Route path="accounting" element={g(<AccountingPage />, ['ketoan'])} />
        <Route path="payouts/:ctvId" element={g(<PayPage />, ['ketoan'])} />
        <Route path="batches" element={g(<PaymentsPage />)} />
        <Route path="batches/:id" element={g(<BatchPage />)} />
        <Route path="reports" element={g(<ReportPage />)} />
        <Route path="me" element={g(<AccountPage />, [...STAFF, 'sale'])} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
