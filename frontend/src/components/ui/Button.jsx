import Loader from './Loader'

/** variant: 'primary' | 'secondary' | 'ghost' */
export default function Button({
  children,
  variant = 'primary',
  type = 'button',
  loading = false,
  disabled = false,
  className = '',
  ...props
}) {
  return (
    <button
      type={type}
      className={`btn btn-${variant} ${className}`.trim()}
      disabled={disabled || loading}
      {...props}
    >
      {loading && <Loader size="sm" />}
      {children}
    </button>
  )
}
