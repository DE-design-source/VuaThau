// Menu theo vai trò. roles: ai thấy mục này. badge: mục ứng với số việc chờ `todo` của GET /me. icon: tên trong Icon.jsx.
// group: nhóm trên thanh bên.
const navLinks = [
  { to: '/queue', label: 'Duyệt công trình', icon: 'queue', roles: ['admin'], badge: true, group: 'Vận hành' },
  { to: '/accounting', label: 'Thanh toán', icon: 'wallet', roles: ['ketoan'], badge: true, group: 'Vận hành' },
  { to: '/houses', label: 'Công trình', icon: 'houses', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/map', label: 'Bản đồ', icon: 'map', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/quy-hoach', label: 'Xem quy hoạch', icon: 'layers', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/ctvs', label: 'Cộng tác viên', icon: 'users', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/contractors', label: 'Nhà thầu', icon: 'staff', roles: ['admin', 'ketoan'], group: 'Vận hành' },
  { to: '/batches', label: 'Phiếu chi', icon: 'receipt', roles: ['admin', 'ketoan'], group: 'Tài chính' },
  { to: '/reports', label: 'Báo cáo', icon: 'chart', roles: ['admin', 'ketoan'], group: 'Tài chính' },
  { to: '/sale/map', label: 'Bản đồ', icon: 'map', roles: ['sale'], group: 'Sale' },
  { to: '/quy-hoach', label: 'Xem quy hoạch', icon: 'layers', roles: ['sale'], group: 'Sale' },
  { to: '/sale/projects', label: 'Dự án', icon: 'houses', roles: ['sale'], group: 'Sale' },
  { to: '/sale/contractors', label: 'Nhà thầu', icon: 'users', roles: ['sale'], group: 'Sale' },
  { to: '/nha-thau-app', label: 'Nhà thầu dùng app', icon: 'houses', roles: ['admin'], group: 'Hệ thống' },
  { to: '/staff', label: 'Nhân viên', icon: 'staff', roles: ['admin'], group: 'Hệ thống' },
  { to: '/settings', label: 'Cấu hình', icon: 'settings', roles: ['admin'], group: 'Hệ thống' },
]

// Người chưa đăng nhập (góp ý 20/11/2026): xem được Bản đồ (công khai) và Xem quy hoạch.
// Góp ý 23/11/2026: ẩn hẳn nhóm "Cần đăng nhập" (không hiện mục khoá); muốn xem thêm thì bấm Đăng nhập.
export const guestLinks = [
  { to: '/ban-do', label: 'Bản đồ', icon: 'map', group: 'Xem tự do' },
  { to: '/quy-hoach', label: 'Xem quy hoạch', icon: 'layers', group: 'Xem tự do' },
]

export default navLinks
