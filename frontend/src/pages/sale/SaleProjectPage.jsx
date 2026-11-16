// Chi tiết 1 dự án cho sale (/sale/projects/:id): thông tin công trình, nhà thầu, vị trí, ảnh. Không có CTV, không có tiền.
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import saleApi from '@/api/saleApi'
import { toast } from '@/services/toast'
import { Button, Icon, PageHead, PageState, PhotoGrid } from '@/components/ui'
import { copyGmap, gmapUrl, hsId, vnDate } from '@/utils/format'

function Fact({ label, children }) {
  return (
    <div className="field">
      <span className="muted">{label}</span>
      <div>{children || <span className="muted">—</span>}</div>
    </div>
  )
}

export default function SaleProjectPage() {
  const { id } = useParams()
  const { data: p, err, reload } = usePage(() => saleApi.project(id), [id])
  if (!p) return <PageState err={err} retry={reload} />
  // PhotoGrid đọc exif_time / lat / lng
  const ps = (p.photo_list || []).map((x) => ({ id: x.id, exif_time: x.time, lat: x.lat, lng: x.lng }))

  return (
    <section className="stack">
      <PageHead title={`Dự án ${hsId(p.id)}`} sub={p.address || '(chưa có địa chỉ)'}>
        <Link className="btn btn-secondary" to="/sale/projects">
          <Icon name="left" size={16} /> Danh sách dự án
        </Link>
      </PageHead>
      <div className="card stack">
        <h2>Thông tin công trình</h2>
        <div className="grid-2 tight">
          <Fact label="Địa chỉ">{p.address}</Fact>
          <Fact label="Tỉnh, thành">{p.province_name}</Fact>
          <Fact label="Giai đoạn">{p.stage}</Fact>
          <Fact label="Số tầng">{p.floors != null ? String(p.floors) : ''}</Fact>
          <Fact label="Nhà thầu">{p.contractor_name}</Fact>
          <Fact label="SĐT nhà thầu">
            {p.contractor_phone && (
              <a className="mono" href={`tel:${p.contractor_phone}`}>
                {p.contractor_phone}
              </a>
            )}
          </Fact>
          <Fact label="Ngày chụp">{vnDate(p.at)}</Fact>
          <Fact label="Vị trí">
            {p.lat != null && (
              <span className="row-wrap">
                <a className="btn btn-secondary" href={gmapUrl(p.lat, p.lng)} target="_blank" rel="noreferrer">
                  <Icon name="pin" size={16} /> Xem Google Maps
                </a>
                <Button variant="ghost" onClick={() => copyGmap(p.lat, p.lng, toast)}>
                  Chép link
                </Button>
              </span>
            )}
          </Fact>
        </div>
      </div>
      <div className="card stack">
        <h2>Ảnh công trình ({ps.length})</h2>
        {ps.length ? <PhotoGrid ps={ps} checks={false} /> : <p className="muted">Chưa có ảnh.</p>}
      </div>
    </section>
  )
}
