// Bản đồ cho sale (/sale/map): bản đồ (thanh tìm địa chỉ phía trên) bên trái; lọc Tỉnh thành, Cộng tác viên +
// danh sách công trình (link Google Maps) bên phải; số liệu cuối trang. Sale thấy tên CTV để lọc; không thấy SĐT, tiền.
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import saleApi from '@/api/saleApi'
import { Button, Kpi, PageHead, PageState } from '@/components/ui'
import SiteList from '@/components/SiteList'
import { AGE_FRESH, AGE_FRESH_LINE, AGE_STALE, ageTip, esc, gmapUrl, hsId, isStale, vnDate } from '@/utils/format'
import { SiteMap } from '@/pages/MapPage'

export default function SaleMapPage() {
  const [sp, setSp] = useSearchParams()
  const cur = { province: sp.get('province') || '', ctv: sp.get('ctv') || '' }
  const { data: d, err, busy, reload } = usePage(() => saleApi.map(cur), [cur.province, cur.ctv])
  const [focus, setFocus] = useState(null)
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const provinces = d.provinces || []
  const ctvs = d.ctvs || []
  const stale = d.stale_days || 30
  const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
  const prov = provinces.find((p) => p.code === cur.province)
  const ctv = ctvs.find((c) => String(c.id) === cur.ctv)
  const withSites = provinces.filter((p) => p.total > 0)
  const set = (patch) => {
    const next = { ...cur, ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }

  const pop = (p) =>
    `<div class="map-pop"><span class="mono">${hsId(p.id)}</span>${p.stage ? ` · ${esc(p.stage)}` : ''}` +
    `<br><b>${esc(p.address || '(chưa có địa chỉ)')}</b>` +
    `${p.province ? `<br>${esc(name[p.province] || '')}` : ''}` +
    `${p.contractor ? `<br>Nhà thầu: ${esc(p.contractor)}` : ''}` +
    `${p.ctv_name ? `<br>CTV: <b class="ctv-name">${esc(p.ctv_name)}</b>` : ''}` +
    `<br><span class="muted mono">Chụp lần cuối ${esc(vnDate(p.last_at || p.at))}</span>` +
    `${isStale(p.last_at, stale) ? '<br><b class="text-ok">Cần cập nhật công trình</b>' : ''}` +
    `<br><a href="/sale/projects/${p.id}" data-to="/sale/projects/${p.id}">Xem dự án</a>` +
    ` · <a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`

  return (
    <section className="stack">
      <PageHead title="Bản đồ công trình" sub="Công trình đang xây trên 34 tỉnh, thành phố. Tìm theo địa chỉ, lọc theo tỉnh hoặc cộng tác viên; bấm 1 công trình để xem trên bản đồ." />

      {prov && prov.merged?.length > 0 && (
        <div className="note tone-wait">
          {prov.name} từ 1/7/2025 gồm: {prov.merged.join(', ')}.
        </div>
      )}

      <div className="map-grid">
        <div className="card map-card">
          <SiteMap
            points={points}
            provinces={provinces}
            selected={cur.province}
            onProvince={(code) => set({ province: code === cur.province ? '' : code })}
            fitKey={`${cur.province}|${cur.ctv}`}
            pop={pop}
            color={(p) => (isStale(p.last_at, stale) ? AGE_STALE : AGE_FRESH)}
            stroke={(p) => (isStale(p.last_at, stale) ? '#fff' : AGE_FRESH_LINE)}
            tip={(p) => ageTip(p.last_at, stale)}
            staleDays={stale}
            focus={focus}
          />
          <div className="map-legend">
            <span>
              <i style={{ background: AGE_FRESH, boxShadow: `0 0 0 1px ${AGE_FRESH_LINE}` }} /> Chụp chưa quá {stale} ngày
            </span>
            <span>
              <i style={{ background: AGE_STALE }} /> Quá {stale} ngày: cần cập nhật công trình
            </span>
          </div>
        </div>

        <div className="card map-aside">
          <div className="aside-filters">
            <select className="input" value={cur.province} onChange={(e) => set({ province: e.target.value })} aria-label="Tỉnh, thành">
              <option value="">Tỉnh thành: cả nước</option>
              {[...provinces]
                .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
                .map((p) => (
                  <option key={p.code} value={p.code}>
                    {p.name} ({p.total})
                  </option>
                ))}
            </select>
            <select className="input" value={cur.ctv} onChange={(e) => set({ ctv: e.target.value })} aria-label="Cộng tác viên">
              <option value="">Cộng tác viên: tất cả</option>
              {ctvs.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} ({c.total})
                </option>
              ))}
            </select>
            {(busy || cur.province || cur.ctv) && (
              <div className="aside-filters-foot">
                {busy ? <span className="muted">Đang tải…</span> : <span />}
                {(cur.province || cur.ctv) && (
                  <Button variant="ghost" type="button" onClick={() => setSp({})}>
                    Bỏ lọc
                  </Button>
                )}
              </div>
            )}
          </div>
          <div className="aside-head">
            Danh sách công trình <b className="mono">{points.length}</b>
          </div>
          <div className="aside-body">
            <SiteList
              points={points}
              staleDays={stale}
              active={focus?.id}
              onPick={(p) => setFocus({ id: p.id, n: (focus?.n || 0) + 1 })}
              meta={(p) => (
                <>
                  {name[p.province] || 'Chưa rõ tỉnh'}
                  {p.contractor ? ` · ${p.contractor}` : ''}
                  {p.ctv_name ? (
                    <>
                      {' · CTV '}
                      <b className="ctv-name">{p.ctv_name}</b>
                    </>
                  ) : null}
                </>
              )}
            />
          </div>
        </div>
      </div>

      <div className="kpis">
        <Kpi label="Công trình trên bản đồ" value={points.length} sub={[prov?.name, ctv?.name].filter(Boolean).join(' · ') || 'cả nước'} />
        <Kpi label="Cần cập nhật" value={points.filter((p) => isStale(p.last_at, stale)).length} sub={`quá ${stale} ngày`} />
        <Kpi label="Tỉnh, thành có công trình" value={`${withSites.length}/34`} />
        <Kpi label="Cộng tác viên đã chụp" value={ctvs.length} />
      </div>
    </section>
  )
}
