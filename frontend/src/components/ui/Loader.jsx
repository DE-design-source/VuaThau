/** size: 'sm' | 'md' | 'lg' */
export default function Loader({ size = 'md', text }) {
  return (
    <span className="loader" role="status" aria-live="polite">
      <span className={`spinner spinner-${size}`} aria-hidden="true" />
      {text ? <span>{text}</span> : <span className="sr-only">Đang tải...</span>}
    </span>
  )
}
