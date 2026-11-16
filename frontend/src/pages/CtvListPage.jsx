// Danh sách cộng tác viên (/ctv): tìm theo tên, SĐT, email; số hồ sơ, chờ duyệt, đã duyệt, đã trả, còn nợ.
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import { Button, Empty, PageHead, PageState, Pager, Pill } from '@/components/ui'
import { qs } from '@/api/ntClient'
import { money, vnDate } from '@/utils/format'

export default function CtvListPage() {
  const [sp, setSp] = useSearchParams()
  const search = sp.toString()
  const { data: d, err, busy, reload } = usePage(`/ctv${search ? `?${search}` : ''}`)
  const [q, setQ] = useState(sp.get('q') || '')
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []

  return (
    <section className="stack">
      <PageHead title="Cộng tác viên" sub={`${d.found} người. CTV tự đăng ký trên app.`} />
      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          setSp(qs({ q }).slice(1))
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
                      <Link to={`/ctv/${r.id}`}>
                        <strong>{r.name}</strong>
                      </Link>{' '}
                      {r.locked === 1 && <Pill tone="bad">Đã khoá</Pill>}
                      {r.locked === 2 && <Pill>Đã xoá</Pill>}
                      <small className="muted d-block">
                        {r.phone}
                        {r.email ? ` · ${r.email}` : ''}
                      </small>
                    </td>
                    <td className="num">{r.n}</td>
                    <td className="num">{r.wait}</td>
                    <td className="num">{r.ok}</td>
                    <td className="num">{money(r.paid)}</td>
                    <td className="num">{money(r.owed)}</td>
                    <td className="muted">{vnDate(r.last) || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="Không tìm thấy cộng tác viên" />
        )}
        <Pager trang={d.trang} pages={d.pages} onGo={(t) => setSp(qs({ q: d.q, trang: t }).slice(1))} />
      </div>
    </section>
  )
}
