// Danh sách dự án cho sale (/sale/projects): địa chỉ, tỉnh, nhà thầu, giai đoạn, ảnh. Không có CTV, không có tiền.
// ?contractor=<khoá>&cname=<tên>: chỉ dự án của 1 nhà thầu (từ trang Nhà thầu).
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useCfg from '@/hooks/useCfg'
import saleApi from '@/api/saleApi'
import { toast } from '@/services/toast'
import { Button, Cover, Empty, PageHead, PageState, Pager, Pill } from '@/components/ui'
import { copyGmap, gmapUrl, hsId, vnDate } from '@/utils/format'

export default function SaleProjectsPage() {
  const [sp, setSp] = useSearchParams()
  const cur = {
    q: sp.get('q') || '',
    province: sp.get('province') || '',
    contractor: sp.get('contractor') || '',
    cname: sp.get('cname') || '',
    page: sp.get('page') || '',
  }
  const { data: d, err, busy, reload } = usePage(() => saleApi.projects(cur), [cur.q, cur.province, cur.contractor, cur.page])
  const cfg = useCfg()
  const [q, setQ] = useState(cur.q)
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []
  const go = (patch) => {
    const next = { ...cur, page: '', ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }

  return (
    <section className="stack">
      <PageHead title="Dự án" sub={`${d.found} công trình`} />
      <form
        className="card filters"
        onSubmit={(e) => {
          e.preventDefault()
          go({ q })
        }}
      >
        <input className="input" placeholder="Tìm địa chỉ, nhà thầu, SĐT nhà thầu, mã dự án" value={q} onChange={(e) => setQ(e.target.value)} />
        <select className="input" style={{ flex: '0 1 230px' }} value={cur.province} onChange={(e) => go({ province: e.target.value })} aria-label="Tỉnh, thành">
          <option value="">Cả nước (34 tỉnh, thành)</option>
          {[...(cfg?.provinces || [])]
            .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
            .map((p) => (
              <option key={p.code} value={p.code}>
                {p.name}
              </option>
            ))}
        </select>
        <Button type="submit" loading={busy}>
          Tìm
        </Button>
        {(cur.q || cur.province || cur.contractor) && (
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
      {cur.contractor && (
        <div className="note tone-wait">
          Đang xem dự án của nhà thầu <b>{cur.cname || cur.contractor}</b>.{' '}
          <a href="#tat-ca" onClick={(e) => (e.preventDefault(), go({ contractor: '', cname: '' }))}>
            Xem tất cả
          </a>
        </div>
      )}

      {rows.length ? (
        <div className="card table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Dự án</th>
                <th>Tỉnh, thành</th>
                <th>Nhà thầu</th>
                <th>Giai đoạn</th>
                <th>Ngày chụp</th>
                <th>Vị trí</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td>
                    <Link to={`/sale/projects/${r.id}`} className="row-link">
                      <Cover id={r.cover_id} />
                      <span>
                        <span className="id">{hsId(r.id)}</span> <Pill>{r.photos} ảnh</Pill>
                        <span className="d-block row-title">{r.address || <span className="muted">(chưa có địa chỉ)</span>}</span>
                      </span>
                    </Link>
                  </td>
                  <td>{r.province_name || '—'}</td>
                  <td>
                    {r.contractor_name || <span className="muted">—</span>}
                    {r.contractor_phone && (
                      <a className="d-block mono" href={`tel:${r.contractor_phone}`}>
                        {r.contractor_phone}
                      </a>
                    )}
                  </td>
                  <td>
                    {r.stage || '—'}
                    {r.floors != null && <small className="muted d-block">{r.floors} tầng</small>}
                  </td>
                  <td className="mono">{vnDate(r.at)}</td>
                  <td>
                    {r.lat != null ? (
                      <>
                        <a href={gmapUrl(r.lat, r.lng)} target="_blank" rel="noreferrer">
                          Google Maps
                        </a>
                        <a className="d-block" href="#chep-link" onClick={(e) => (e.preventDefault(), copyGmap(r.lat, r.lng, toast))}>
                          Chép link
                        </a>
                      </>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty title="Không có dự án nào" text="Thử bỏ bớt bộ lọc." />
      )}
      <Pager page={d.page} pages={d.pages} onGo={(p) => go({ page: p > 1 ? String(p) : '' })} />
    </section>
  )
}
