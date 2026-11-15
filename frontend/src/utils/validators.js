// Mirrors backend validation rules:
// username 3-50 chars, valid email, password 8-72 chars.

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export const isValidEmail = (value = '') => EMAIL_REGEX.test(value.trim())

/** Each validator returns an error message, or '' when valid. */
export function validateUsername(value = '') {
  const length = value.trim().length
  if (!length) return 'Vui lòng nhập tên người dùng'
  if (length < 3 || length > 50) return 'Tên người dùng phải từ 3 đến 50 ký tự'
  return ''
}

export function validateEmail(value = '') {
  if (!value.trim()) return 'Vui lòng nhập email'
  if (!isValidEmail(value)) return 'Email không hợp lệ'
  return ''
}

export function validatePassword(value = '') {
  if (!value) return 'Vui lòng nhập mật khẩu'
  if (value.length < 8 || value.length > 72) return 'Mật khẩu phải từ 8 đến 72 ký tự'
  return ''
}

const compact = (errors) => Object.fromEntries(Object.entries(errors).filter(([, msg]) => msg))

/** Returns an object of field -> message; empty object means valid. */
export const validateLoginForm = ({ email, password }) =>
  compact({ email: validateEmail(email), password: validatePassword(password) })

export const validateRegisterForm = ({ username, email, password }) =>
  compact({
    username: validateUsername(username),
    email: validateEmail(email),
    password: validatePassword(password),
  })
