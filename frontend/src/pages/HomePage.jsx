import { useEffect, useMemo, useState } from 'react'
import userApi from '@/api/userApi'
import useAuth from '@/hooks/useAuth'
import useDebounce from '@/hooks/useDebounce'
import useWebSocket from '@/hooks/useWebSocket'
import { Input, Loader } from '@/components/ui'
import { formatDateTime } from '@/utils/formatDate'

export default function HomePage() {
  const { user } = useAuth()
  const [users, setUsers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [incoming, setIncoming] = useState([])
  const debouncedSearch = useDebounce(search, 300)

  const { connected } = useWebSocket((event) => {
    if (event.type === 'message:new') setIncoming((prev) => [event.data, ...prev].slice(0, 20))
  })

  useEffect(() => {
    let ignore = false
    userApi
      .list({ limit: 50 })
      .then((res) => !ignore && setUsers(res.data ?? []))
      .catch((err) => !ignore && setError(err.message))
      .finally(() => !ignore && setLoading(false))
    return () => {
      ignore = true
    }
  }, [])

  const filteredUsers = useMemo(() => {
    const keyword = debouncedSearch.trim().toLowerCase()
    if (!keyword) return users
    return users.filter(
      // email is only returned for the current user, so it may be missing.
      (u) => u.username.toLowerCase().includes(keyword) || (u.email ?? '').toLowerCase().includes(keyword),
    )
  }, [users, debouncedSearch])

  return (
    <div className="stack">
      <section className="card">
        <h1>Xin chào{user ? `, ${user.username}` : ''}!</h1>
        <p className="muted">
          Realtime: {connected ? 'đã kết nối' : 'chưa kết nối'}
          {user && ` · Tham gia ${formatDateTime(user.created_at)}`}
        </p>
      </section>

      <section className="card stack">
        <h2>Người dùng</h2>
        <Input
          placeholder="Tìm theo tên..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        {loading && <Loader text="Đang tải..." />}
        {error && <p className="field-error">{error}</p>}
        {!loading && !error && (
          <ul className="list">
            {filteredUsers.map((u) => (
              <li key={u.id}>
                <strong>{u.username}</strong>
                {u.email && <span className="muted"> {u.email}</span>}
              </li>
            ))}
            {filteredUsers.length === 0 && <li className="muted">Không có kết quả</li>}
          </ul>
        )}
      </section>

      <section className="card stack">
        <h2>Tin nhắn mới</h2>
        {incoming.length === 0 ? (
          <p className="muted">Chưa có tin nhắn nào.</p>
        ) : (
          <ul className="list">
            {incoming.map((msg) => (
              <li key={msg.id}>
                <span className="muted">{formatDateTime(msg.created_at)}</span> — {msg.content}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
