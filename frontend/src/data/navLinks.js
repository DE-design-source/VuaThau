// Static navigation config used by the Header.
// `auth: true` -> only shown when logged in, `guest: true` -> only when logged out.
const navLinks = [
  { to: '/', label: 'Trang chủ', end: true },
  { to: '/login', label: 'Đăng nhập', guest: true },
]

export default navLinks
