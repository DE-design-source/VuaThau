// Menu theo vai trò. roles: ai thấy mục này. badge: mục ứng với số việc chờ `todo` của GET /me. icon: tên trong Icon.jsx.
// group: nhóm trên thanh bên.
const navLinks = [
  { to: '/queue', label: 'Duyệt hồ sơ', icon: 'queue', roles: ['admin'], badge: true, group: 'Vận hành' },
  { to: '/accounting', label: 'Thanh toán', icon: 'wallet', roles: ['ketoan'], badge: true, group: 'Vận hành' },
  { to: '/houses', label: 'Hồ sơ', icon: 'houses', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/map', label: 'Bản đồ', icon: 'map', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/quy-hoach', label: 'Xem quy hoạch', icon: 'layers', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/ctvs', label: 'Cộng tác viên', icon: 'users', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/batches', label: 'Phiếu chi', icon: 'receipt', roles: ['admin', 'ketoan'], group: 'Tài chính' },
  { to: '/reports', label: 'Báo cáo', icon: 'chart', roles: ['admin', 'ketoan'], group: 'Tài chính' },
  { to: '/sale/map', label: 'Bản đồ', icon: 'map', roles: ['sale'], group: 'Sale' },
  { to: '/quy-hoach', label: 'Xem quy hoạch', icon: 'layers', roles: ['sale'], group: 'Sale' },
  { to: '/sale/projects', label: 'Dự án', icon: 'houses', roles: ['sale'], group: 'Sale' },
  { to: '/sale/contractors', label: 'Nhà thầu', icon: 'users', roles: ['sale'], group: 'Sale' },
  { to: '/staff', label: 'Nhân viên', icon: 'staff', roles: ['admin'], group: 'Hệ thống' },
  { to: '/settings', label: 'Cấu hình', icon: 'settings', roles: ['admin'], group: 'Hệ thống' },
]

// Người chưa đăng nhập (góp ý 20/11/2026): xem được Bản đồ (công khai) và Xem quy hoạch; mục khác hiện ổ khoá,
// bấm vào thì sang Đăng nhập.
export const guestLinks = [
  { to: '/ban-do', label: 'Bản đồ', icon: 'map', group: 'Xem tự do' },
  { to: '/quy-hoach', label: 'Xem quy hoạch', icon: 'layers', group: 'Xem tự do' },
  { to: '/houses', label: 'Hồ sơ', icon: 'houses', group: 'Cần đăng nhập', locked: true },
  { to: '/ctvs', label: 'Cộng tác viên', icon: 'users', group: 'Cần đăng nhập', locked: true },
  { to: '/sale/projects', label: 'Dự án', icon: 'houses', group: 'Cần đăng nhập', locked: true },
  { to: '/sale/contractors', label: 'Nhà thầu', icon: 'users', group: 'Cần đăng nhập', locked: true },
  { to: '/batches', label: 'Phiếu chi', icon: 'receipt', group: 'Cần đăng nhập', locked: true },
  { to: '/reports', label: 'Báo cáo', icon: 'chart', group: 'Cần đăng nhập', locked: true },
]

export default navLinks
