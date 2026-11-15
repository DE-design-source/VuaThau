import { Link } from 'react-router-dom'

export default function NotFoundPage() {
  return (
    <section className="card center">
      <h1>404</h1>
      <p className="muted">Không tìm thấy trang bạn yêu cầu.</p>
      <Link to="/">Về trang chủ</Link>
    </section>
  )
}
