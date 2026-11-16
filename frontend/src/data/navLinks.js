// Menu theo vai trò. roles: ai thấy mục này. badge: mục ứng với số việc chờ `todo` của GET /me.
const navLinks = [
  { to: '/queue', label: 'Duyệt hồ sơ', roles: ['admin'], badge: true },
  { to: '/accounting', label: 'Thanh toán', roles: ['ketoan'], badge: true },
  { to: '/houses', label: 'Hồ sơ', roles: ['admin', 'ketoan'] },
  { to: '/ctvs', label: 'Cộng tác viên', roles: ['admin', 'ketoan'] },
  { to: '/map', label: 'Bản đồ', roles: ['admin', 'ketoan'] },
  { to: '/batches', label: 'Phiếu chi', roles: ['admin', 'ketoan'] },
  { to: '/reports', label: 'Báo cáo', roles: ['admin', 'ketoan'] },
  { to: '/staff', label: 'Nhân viên', roles: ['admin'] },
  { to: '/settings', label: 'Cấu hình', roles: ['admin'] },
]

export default navLinks
