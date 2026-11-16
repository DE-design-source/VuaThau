// Nhà thầu cho sale (/sale/contractors): gộp từ các công trình (theo SĐT, không có SĐT thì theo tên).
// Bấm 1 nhà thầu để xem các dự án của họ.
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import saleApi from '@/api/saleApi'
import { Button, Empty, PageHead, PageState } from '@/components/ui'
import { vnDate } from '@/utils/format'

export default function ContractorsPage() {
  const [sp, setSp] = useSearchParams()
  const cur = sp.get('q') || ''
  const { data: d, err, busy, reload } = usePage(() => saleApi.contractors({ q: cur }), [cur])
  const [q, setQ] = useState(cur)
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []

  return (
    <section className="stack">
      <PageHead title="Nhà thầu" sub={`${rows.length} nhà thầu, lấy từ bảng hiệu / bạt che tại công trình`} />
      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          setSp(q ? { q } : {})
        }}
      >
        <input className="input" placeholder="Tìm tên, SĐT nhà thầu, địa chỉ công trình" value={q} onChange={(e) => setQ(e.target.value)} />
        <Button type="submit" loading={busy}>
          Tìm
        </Button>
        {cur && (
          <Button
            variant="ghost"
            type="button"
            onClick={() => {
              setQ('')
              setSp({})
            }}
          >
            Bỏ lọc
          </Button>
        )}
      </form>
      {rows.length ? (
        <div className="card table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Nhà thầu</th>
                <th>SĐT</th>
                <th className="num">Công trình</th>
                <th>Tỉnh, thành</th>
                <th>Gần nhất</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((c) => (
                <tr key={c.key}>
                  <td>
                    <Link to={`/sale/projects?${new URLSearchParams({ contractor: c.key, cname: c.name || c.phone })}`}>
                      <strong>{c.name || '(không có tên)'}</strong>
                    </Link>
                  </td>
                  <td>
                    {c.phone ? (
                      <a className="mono" href={`tel:${c.phone}`}>
                        {c.phone}
                      </a>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td className="num">
                    <strong>{c.total}</strong>
                  </td>
                  <td>{c.provinces?.length ? c.provinces.join(', ') : '—'}</td>
                  <td className="mono">{vnDate(c.last)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty title="Chưa có nhà thầu nào" />
      )}
    </section>
  )
}
