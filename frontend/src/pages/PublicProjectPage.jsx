// Chi tiết 1 công trình cho người chưa đăng nhập (/ban-do/:id, góp ý 23/11/2026: bản free bấm vào xem được chi tiết).
// Thông tin công trình + ảnh (đã che SĐT, link có chữ ký). Không SĐT nhà thầu, không CTV, không tiền.
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import publicApi from '@/api/publicApi'
import { Icon, PageHead, PageState } from '@/components/ui'
import { agoLabel, gmapUrl, hsId, vnDate } from '@/utils/format'

function Fact({ label, children }) {
  return (
    <div className="field">
      <span className="muted">{label}</span>
      <div>{children || <span className="muted">—</span>}</div>
    </div>
  )
}

export default function PublicProjectPage() {
  const { id } = useParams()
  const { data: p, err, reload } = usePage(() => publicApi.project(id), [id])
  const [big, setBig] = useState(-1)
  if (!p) return <PageState err={err} retry={reload} />
  const ps = p.photos || []
  return (
    <section className="stack">
      <PageHead title={p.address || `Công trình ${hsId(p.id)}`} sub={[p.province_name, p.stage, `chụp ${agoLabel(p.at).toLowerCase()}`].filter(Boolean).join(' · ')}>
        <Link className="btn btn-secondary" to="/ban-do">
          <Icon name="left" size={16} /> Bản đồ
        </Link>
      </PageHead>
      <div className="card stack">
        <h2>Ảnh công trình ({ps.length})</h2>
        {ps.length ? (
          <div className="pub-photos">
            {ps.map((x, i) => (
              <button type="button" key={x.url} onClick={() => setBig(i)} aria-label={`Xem ảnh ${i + 1}`}>
                <img src={x.url} alt="" loading="lazy" />
              </button>
            ))}
          </div>
        ) : (
          <p className="muted">Chưa có ảnh.</p>
        )}
      </div>
      <div className="card stack">
        <h2>Thông tin công trình</h2>
        <div className="grid-2 tight">
          <Fact label="Địa chỉ">{p.address}</Fact>
          <Fact label="Tỉnh, thành">{p.province_name}</Fact>
          <Fact label="Giai đoạn">{p.stage}</Fact>
          <Fact label="Số tầng">{p.floors != null ? String(p.floors) : ''}</Fact>
          <Fact label="Nhà thầu">{p.contractor}</Fact>
          <Fact label="Ngày chụp">{vnDate(p.at)}</Fact>
          <Fact label="Vị trí">
            {p.lat != null && (
              <a className="btn btn-secondary" href={gmapUrl(p.lat, p.lng)} target="_blank" rel="noreferrer">
                <Icon name="pin" size={16} /> Xem Google Maps
              </a>
            )}
          </Fact>
        </div>
        <p className="muted">
          Xem SĐT nhà thầu, nhật ký thi công và báo cáo: <Link to="/login">đăng nhập</Link>.
        </p>
      </div>
      {big >= 0 && ps[big] && (
        <div className="pub-viewer" role="dialog" aria-label="Xem ảnh" onClick={() => setBig(-1)}>
          <img src={ps[big].url} alt="" />
          <span className="pub-viewer-n">
            {big + 1}/{ps.length} · bấm để đóng
          </span>
        </div>
      )}
    </section>
  )
}
