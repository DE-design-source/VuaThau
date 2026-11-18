// Nhà thầu cho sale (/sale/contractors), 2 nguồn:
//  - Hệ thống: gộp từ các công trình (theo SĐT, không có SĐT thì theo tên); bấm để xem các dự án của họ.
//  - Dezon: danh sách "Nhà thầu tiềm năng" trên dezon.vn; bấm mở trang chi tiết trên dezon.vn (tab mới).
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import saleApi from '@/api/saleApi'
import { Button, Empty, PageHead, PageState } from '@/components/ui'
import { vnDate, vnTime } from '@/utils/format'

const fold = (s) =>
  (s || '')
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/đ/g, 'd')

export default function ContractorsPage() {
  const [sp, setSp] = useSearchParams()
  const src = sp.get('src') === 'dezon' ? 'dezon' : 'system'
  const tab = (k) => (
    <button type="button" className={`chip ${src === k ? 'chip-on' : ''}`} onClick={() => setSp(k === 'dezon' ? { src: k } : {})}>
      {k === 'dezon' ? 'Dezon · nhà thầu tiềm năng' : 'Hệ thống Vua Thầu'}
    </button>
  )
  return (
    <section className="stack">
      <div className="chips">
        {tab('system')}
        {tab('dezon')}
      </div>
      {src === 'dezon' ? <DezonView /> : <SystemView sp={sp} setSp={setSp} />}
    </section>
  )
}

function SystemView({ sp, setSp }) {
  const cur = sp.get('q') || ''
  const { data: d, err, busy, reload } = usePage(() => saleApi.contractors({ q: cur }), [cur])
  const [q, setQ] = useState(cur)
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []

  return (
    <>
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
    </>
  )
}

function DezonView() {
  const { data: d, err, reload } = usePage(() => saleApi.dezonContractors(), [])
  const [q, setQ] = useState('')
  const [tag, setTag] = useState('')
  const all = useMemo(() => d?.rows || [], [d])
  const tags = useMemo(() => [...new Set(all.flatMap((c) => c.tags || []))], [all])
  const rows = useMemo(() => {
    const k = fold(q.trim())
    return all.filter(
      (c) =>
        (!tag || (tag === '*' ? c.featured : (c.tags || []).includes(tag))) &&
        (!k || fold([c.name, c.province, ...(c.tags || [])].join(' ')).includes(k)),
    )
  }, [all, q, tag])
  if (!d) return <PageState err={err} retry={reload} />

  return (
    <>
      <PageHead
        title="Nhà thầu tiềm năng trên Dezon"
        sub={
          <>
            {all.length} nhà thầu, lấy từ{' '}
            <a href={d.source} target="_blank" rel="noopener noreferrer">
              dezon.vn/nha-thau/tiem-nang
            </a>{' '}
            lúc {vnTime(d.at)}
            {d.stale ? ' (dezon.vn đang lỗi, đây là bản đọc được gần nhất)' : ''}. Bấm vào nhà thầu để xem hồ sơ trên Dezon.
          </>
        }
      />
      <div className="card filters">
        <input className="input" placeholder="Tìm tên nhà thầu, tỉnh thành, loại công trình" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <div className="chips">
        <button type="button" className={`chip ${!tag ? 'chip-on' : ''}`} onClick={() => setTag('')}>
          Tất cả <b>{all.length}</b>
        </button>
        <button type="button" className={`chip ${tag === '*' ? 'chip-on' : ''}`} onClick={() => setTag('*')}>
          Đề xuất <b>{all.filter((c) => c.featured).length}</b>
        </button>
        {tags.map((t) => (
          <button key={t} type="button" className={`chip ${tag === t ? 'chip-on' : ''}`} onClick={() => setTag(t)}>
            {t} <b>{all.filter((c) => (c.tags || []).includes(t)).length}</b>
          </button>
        ))}
      </div>
      {rows.length ? (
        <div className="dz-grid">
          {rows.map((c) => (
            <a key={c.url} className="dz-card" href={c.url} target="_blank" rel="noopener noreferrer">
              <span className="dz-cover">
                {c.cover ? <img src={c.cover} alt="" loading="lazy" referrerPolicy="no-referrer" /> : <span className="dz-noimg">{c.name.slice(0, 1)}</span>}
                {c.featured && <span className="dz-badge">Đề xuất</span>}
              </span>
              <span className="dz-body">
                <strong>{c.name}</strong>
                <span className="muted">
                  {[c.province, c.projects ? `${c.projects} công trình` : '', c.years ? `${c.years} năm` : ''].filter(Boolean).join(' · ')}
                </span>
                {c.tags?.length > 0 && (
                  <span className="dz-tags">
                    {c.tags.map((t) => (
                      <span key={t}>{t}</span>
                    ))}
                  </span>
                )}
                <span className="dz-go">Xem hồ sơ trên Dezon ↗</span>
              </span>
            </a>
          ))}
        </div>
      ) : (
        <Empty title="Không có nhà thầu nào khớp" />
      )}
    </>
  )
}
