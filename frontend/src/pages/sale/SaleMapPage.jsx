// Bản đồ cho sale (/sale/map): công trình trên nền 34 tỉnh, thành. Không có thông tin CTV, không có tiền.
// Bấm 1 tỉnh (trên bản đồ hoặc bảng bên) để lọc; popup mỗi công trình có địa chỉ, nhà thầu, link dự án và Google Maps.
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import saleApi from '@/api/saleApi'
import { Button, Kpi, PageHead, PageState } from '@/components/ui'
import { AGE_FRESH, AGE_FRESH_LINE, AGE_STALE, ageTip, esc, gmapUrl, hsId, isStale, vnDate } from '@/utils/format'
import { SiteMap } from '@/pages/MapPage'

export default function SaleMapPage() {
  const [sp, setSp] = useSearchParams()
  const province = sp.get('province') || ''
  const { data: d, err, reload } = usePage(() => saleApi.map({ province }), [province])
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const provinces = d.provinces || []
  const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
  const prov = provinces.find((p) => p.code === province)
  const withSites = provinces.filter((p) => p.total > 0)
  const byProvince = [...provinces].sort((a, b) => b.total - a.total || a.name.localeCompare(b.name, 'vi'))
  const stale = d.stale_days || 30
  const pick = (code) => setSp(code && code !== province ? { province: code } : {})

  const pop = (p) =>
    `<div class="map-pop"><span class="mono">${hsId(p.id)}</span>${p.stage ? ` · ${esc(p.stage)}` : ''}` +
    `<br><b>${esc(p.address || '(chưa có địa chỉ)')}</b>` +
    `${p.province ? `<br>${esc(name[p.province] || '')}` : ''}` +
    `${p.contractor ? `<br>Nhà thầu: ${esc(p.contractor)}` : ''}` +
    `<br><span class="muted mono">Chụp lần cuối ${esc(vnDate(p.last_at || p.at))}</span>` +
    `${isStale(p.last_at, stale) ? '<br><b class="text-ok">Cần cập nhật công trình</b>' : ''}` +
    `<br><a href="/sale/projects/${p.id}" data-to="/sale/projects/${p.id}">Xem dự án</a>` +
    ` · <a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`

  return (
    <section className="stack">
      <PageHead title="Bản đồ công trình" sub="Công trình đang xây trên 34 tỉnh, thành phố. Bấm vào 1 tỉnh để lọc, bấm vào chấm để xem dự án." />
      <div className="kpis">
        <Kpi label="Công trình trên bản đồ" value={points.length} sub={prov?.name || 'cả nước'} />
        <Kpi label="Tỉnh, thành có công trình" value={`${withSites.length}/34`} />
      </div>

      <div className="card filters">
        <select className="input" value={province} onChange={(e) => pick(e.target.value)} aria-label="Tỉnh, thành">
          <option value="">Cả nước (34 tỉnh, thành)</option>
          {[...provinces]
            .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
            .map((p) => (
              <option key={p.code} value={p.code}>
                {p.name} ({p.total})
              </option>
            ))}
        </select>
        {province && (
          <Button variant="ghost" type="button" onClick={() => setSp({})}>
            Bỏ lọc
          </Button>
        )}
      </div>

      {prov && prov.merged?.length > 0 && (
        <div className="note tone-wait">
          {prov.name} từ 1/7/2025 gồm: {prov.merged.join(', ')}.
        </div>
      )}

      <div className="map-layout">
        <div className="card map-card">
          <SiteMap
            points={points}
            provinces={provinces}
            selected={province}
            onProvince={pick}
            fitKey={province}
            pop={pop}
            color={(p) => (isStale(p.last_at, stale) ? AGE_STALE : AGE_FRESH)}
            stroke={(p) => (isStale(p.last_at, stale) ? '#fff' : AGE_FRESH_LINE)}
            tip={(p) => ageTip(p.last_at, stale)}
            staleDays={stale}
          />
          <div className="map-legend">
            <span>
              <i style={{ background: AGE_FRESH, boxShadow: `0 0 0 1px ${AGE_FRESH_LINE}` }} /> Chụp chưa quá {stale} ngày
            </span>
            <span>
              <i style={{ background: AGE_STALE }} /> Quá {stale} ngày: cần cập nhật công trình
            </span>
            <span className="spacer" />
            <span>Nền: ranh giới 34 tỉnh, thành (OCHA, 2025) · đậm = nhiều công trình</span>
          </div>
        </div>
        <div className="card map-side">
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Tỉnh, thành</th>
                  <th className="num">Công trình</th>
                </tr>
              </thead>
              <tbody>
                {byProvince.map((p) => (
                  <tr
                    key={p.code}
                    className={`${p.code === province ? 'row-active' : ''} ${p.total ? '' : 'row-off'}`}
                    onClick={() => pick(p.code)}
                    style={{ cursor: 'pointer' }}
                  >
                    <td>
                      <strong>{p.name}</strong>
                      {p.merged?.length > 0 && <small className="muted d-block">gồm {p.merged.join(', ')}</small>}
                    </td>
                    <td className="num">
                      <strong>{p.total}</strong>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </section>
  )
}
