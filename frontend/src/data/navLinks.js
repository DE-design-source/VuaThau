// Menu theo vai trò. roles: ai thấy mục này. badge: số việc chờ máy chủ gửi kèm (badge).
const navLinks = [
  { to: '/duyet', label: 'Duyệt hồ sơ', roles: ['admin'], badge: true },
  { to: '/ke-toan', label: 'Thanh toán', roles: ['ketoan'], badge: true },
  { to: '/tong-quan', label: 'Hồ sơ', roles: ['admin', 'ketoan'] },
  { to: '/ctv', label: 'Cộng tác viên', roles: ['admin', 'ketoan'] },
  { to: '/da-tra', label: 'Phiếu chi', roles: ['admin', 'ketoan'] },
  { to: '/bao-cao', label: 'Báo cáo', roles: ['admin', 'ketoan'] },
  { to: '/nhan-vien', label: 'Nhân viên', roles: ['admin'] },
  { to: '/cau-hinh', label: 'Cấu hình', roles: ['admin'] },
]

export default navLinks
