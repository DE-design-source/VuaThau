// Danh sách cộng tác viên (/ctvs): tìm theo tên, SĐT, email; số hồ sơ, chờ duyệt, đã duyệt, đã trả, còn nợ.
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import ctvApi from '@/api/ctvApi'
import { Button, Empty, PageHead, PageState, Pager, Pill } from '@/components/ui'
import { money, vnDate } from '@/utils/format'

export default function CtvListPage() {
  const [sp, setSp] = useSearchParams()
  const cur = { q: sp.get('q') || '', page: sp.get('page') || '' }
  const { data: d, err, busy, reload } = usePage(() => ctvApi.list(cur), [cur.q, cur.page])
  const [q, setQ] = useState(cur.q)
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []
  const go = ({ q: nq = cur.q, page }) => {
    const next = {}
    if (nq) next.q = nq
    if (page > 1) next.page = String(page)
    setSp(next)
  }

  return (
    <section className="stack">
      <PageHead title="Cộng tác viên" sub={`${d.found} người. CTV tự đăng ký trên app.`} />
      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          go({ q })
        }}
      >
        <input className="input" placeholder="Tìm tên, SĐT, email" value={q} onChange={(e) => setQ(e.target.value)} />
        <Button type="submit" loading={busy}>
          Tìm
        </Button>
      </form>
      <div className="card">
        {rows.length ? (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>CTV</th>
                  <th className="num">Hồ sơ</th>
                  <th className="num">Chờ duyệt</th>
                  <th className="num">Đã duyệt</th>
                  <th className="num">Đã trả</th>
                  <th className="num">Còn nợ</th>
                  <th>Nộp gần nhất</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.id}>
                    <td>
                      <Link to={`/ctvs/${r.id}`}>
                        <strong>{r.name}</strong>
                      </Link>{' '}
                      {r.locked === 1 && <Pill tone="bad">Đã khoá</Pill>}
                      {r.locked === 2 && <Pill>Đã xoá</Pill>}
                      <small className="muted d-block">
                        {r.username}
                        {r.email ? ` · ${r.email}` : ''}
                      </small>
                    </td>
                    <td className="num">{r.houses}</td>
                    <td className="num">{r.waiting}</td>
                    <td className="num">{r.approved}</td>
                    <td className="num">{money(r.paid)}</td>
                    <td className="num">{money(r.owed)}</td>
                    <td className="muted">{vnDate(r.last_at) || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="Không tìm thấy cộng tác viên" />
        )}
        <Pager page={d.page} pages={d.pages} onGo={(p) => go({ page: p })} />
      </div>
    </section>
  )
}
