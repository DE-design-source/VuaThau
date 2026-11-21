// Khung trang Bản đồ kiểu Airbnb (admin + sale):
//   trên: thanh tìm dạng viên thuốc (Địa chỉ | các ô lọc | nút tìm tròn) + nút Bộ lọc;
//   trái: "N công trình" + lưới thẻ ảnh (3 cột trên màn rộng); phải: bản đồ bo góc đứng yên khi cuộn, nhãn mã hồ sơ.
import { useEffect, useMemo, useRef, useState } from 'react'
import SiteMap from './SiteMap'
import SiteCard from './SiteCard'
import { useNavigate } from 'react-router-dom'
import { ctvTip, cusTip, dezonCard } from '@/utils/siteTip'
import { distM, distText, hsId, isStale as stale } from '@/utils/format'
import { CtvList, CustomerList, LayerPanel } from './layers'
import { ctvKey, ctvVisible, useLayers } from '@/utils/mapLayers'

const PAGE = 24
const NONE = []
const SAME_SITE_M = 60 // công trình Dezon cách hồ sơ CTV dưới chừng này: coi là đã có CTV chụp, không hiện riêng

/** Ghim nhỏ trong chú thích (cùng hình với ghim trên bản đồ): '' vàng, 'stale' xanh lá, 'dz' xanh dương. */
function PinMark({ kind = '' }) {
  return (
    <svg className={`pin-mark ${kind}`} viewBox="0 0 32 42" width="14" height="19" aria-hidden="true">
      <path d="M16 1.5C8.3 1.5 2 7.7 2 15.4 2 25.8 16 40.5 16 40.5S30 25.8 30 15.4C30 7.7 23.7 1.5 16 1.5z" fill="currentColor" />
      <circle cx="16" cy="15.4" r="6.2" fill="#fff" />
    </svg>
  )
}

/**
 * points; provinces; staleDays
 * selects: [{ key, label, value, options: [{ value, label }], onChange(v) }] ô lọc trong thanh tìm
 * selectedProvince, onProvince(code): bấm tỉnh trên bản đồ; fitKey: đổi khi đổi bộ lọc (bản đồ canh lại)
 * pop(p): HTML thẻ khi bấm nhãn; card(p): props cho SiteCard (xem cardProps trong utils/siteTip)
 * fresh: Set id mới (thời gian thực); filterPanel: nội dung nút Bộ lọc; filterCount: số bộ lọc đang bật
 * footer: phần cuối danh sách (số liệu...); busy: đang tải
 * dezon: công trình nhà thầu đăng trên dezon.vn: dùng chung với hồ sơ CTV (ghim xanh dương + thẻ trong danh sách),
 *   trừ công trình đã có CTV chụp (cách hồ sơ CTV dưới SAME_SITE_M). Truyền [] khi đang lọc theo CTV / trạng thái.
 * ctvPins (admin / kế toán, layout 21/11/2026): có thì thêm tab Công trình | Cộng tác viên, ghim CTV và bảng Lớp bản đồ;
 *   ô tìm lọc ngay danh sách (địa chỉ, công ty, nhà thầu, CTV, SĐT), Enter thì tìm địa điểm trên bản đồ.
 */
export default function Explorer({ points: allPoints, provinces, staleDays = 30, selects = [], selectedProvince, onProvince, fitKey, pop, card, fresh, filterPanel, filterCount = 0, footer, busy, dezon = NONE, ctvPins, customers, canEditCustomer = false }) {
  const [addr, setAddr] = useState('')
  const layered = Array.isArray(ctvPins)
  const [layers, setLayers] = useLayers()
  const [tab, setTab] = useState('projects')
  const navigate = useNavigate()
  // tìm nhanh trong danh sách (chỉ trang có lớp CTV): bỏ dấu, chữ thường
  const fold = (s) => (s || '').normalize('NFD').replace(/[\u0300-\u036f]/g, '').replace(/đ/g, 'd').replace(/Đ/g, 'd').toLowerCase()
  const q = layered ? fold(addr.trim()) : ''
  const hit = (...xs) => !q || fold(xs.join(' ')).includes(q)
  const digits = q.replace(/\D/g, '')
  const phoneHit = (p) => digits.length >= 4 && (p || '').replace(/\D/g, '').includes(digits)
  const points = layered
    ? allPoints.filter((p) => layers[stale(p.last_at || p.at, staleDays) ? 'shoot' : 'pending'] && hit(p.address, p.contractor, p.ctv_name, p.stage, hsId(p.id)))
    : allPoints
  const search = useRef(null) // MapSearch: search.current.find(chữ)
  const [active, setActive] = useState(null)
  const [focus, setFocus] = useState(null)
  // số thẻ đang hiện, gắn với bộ lọc: đổi bộ lọc thì về trang đầu
  const [more, setMore] = useState({ key: fitKey, n: PAGE })
  const shown = more.key === fitKey ? more.n : PAGE
  const [open, setOpen] = useState(false)
  const [near, setNear] = useState(null) // { name, lat, lng, radius }: chỉ công trình quanh vị trí đã tìm
  const pop$ = useRef(null)

  // Bấm ra ngoài: đóng Bộ lọc
  useEffect(() => {
    if (!open) return
    const close = (e) => !pop$.current?.contains(e.target) && setOpen(false)
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  // Dezon chưa có CTV chụp (không trùng chỗ hồ sơ nào)
  const dzAll = useMemo(
    () => (dezon || NONE).filter((d) => d.lat != null && !allPoints.some((p) => distM(d.lat, d.lng, p.lat, p.lng) < SAME_SITE_M)),
    [dezon, allPoints],
  )
  const dz = layered ? (layers.dezon ? dzAll.filter((d) => hit(d.title, d.contractor, d.address)) : NONE) : dzAll
  // cộng tác viên theo lớp đang bật + ô tìm
  const ctvs = layered ? ctvPins.filter((c) => ctvVisible(c, layers) && (hit(c.name, c.address, c.kind) || phoneHit(c.phone))) : NONE
  const counts = layered
    ? {
        pending: allPoints.filter((p) => !stale(p.last_at || p.at, staleDays)).length,
        shoot: allPoints.filter((p) => stale(p.last_at || p.at, staleDays)).length,
        dezon: dzAll.length,
        ctvUpdate: ctvPins.filter((c) => ctvKey(c) === 'ctvUpdate').length,
        ctvSales: ctvPins.filter((c) => ctvKey(c) === 'ctvSales').length,
        ctvActive: ctvPins.filter((c) => c.status === 'active').length,
        ctvReg: ctvPins.filter((c) => c.status !== 'active').length,
      }
    : null
  // khách hàng: lớp Khách hàng + ô Phụ trách + ô tìm
  const [owner, setOwner] = useState('')
  const allCus = layered ? customers || NONE : NONE
  const cus = layered && layers.customer ? allCus.filter((k) => (!owner || String(k.ctv_id) === owner) && (hit(k.name, k.address, k.ctv_name, k.note) || phoneHit(k.phone))) : NONE
  if (counts) counts.customer = allCus.length
  const pickCus = (k) => {
    if (k.lat == null) {
      if (canEditCustomer) navigate(`/customers?edit=${k.id}`)
      return
    }
    setFocus({ id: `kh${k.id}`, n: (focus?.n || 0) + 1 })
  }
  const pickCtv = (c) => {
    if (c.lat == null) {
      navigate(`/ctvs/${c.id}`) // chưa rõ vị trí: mở hồ sơ CTV
      return
    }
    setFocus({ id: `ctv${c.id}`, n: (focus?.n || 0) + 1 })
  }
  // 1 danh sách chung: hồ sơ CTV (khoá = id) + Dezon (khoá = "dz" + id), mới nhất trước
  const items = [
    ...points.map((p) => ({ key: p.id, p, at: p.last_at || p.at })),
    ...dz.map((d) => ({ key: `dz${d.id}`, d, p: d, at: d.posted_at })),
  ]
  const dist = near ? new Map(items.map((it) => [it.key, distM(near.lat, near.lng, it.p.lat, it.p.lng)])) : null
  const rows = near
    ? items.filter((it) => dist.get(it.key) <= near.radius).sort((a, b) => dist.get(a.key) - dist.get(b.key))
    : items.sort((a, b) => new Date(b.at || 0) - new Date(a.at || 0))
  const provName = Object.fromEntries((provinces || []).map((p) => [p.code, p.name]))

  return (
    <div className="ex">
      <div className="ex-bar">
        <form
          className="ex-search"
          onSubmit={(e) => {
            e.preventDefault()
            search.current?.find(addr)
          }}
        >
          <label className="ex-seg ex-seg-addr">
            <span>Địa chỉ</span>
            <input
              value={addr}
              onChange={(e) => setAddr(e.target.value)}
              placeholder={layered ? 'Tìm địa chỉ, công ty, nhà thầu, CTV, SĐT…' : 'Địa chỉ, công ty, nhà thầu, SĐT…'}
              aria-label="Tìm địa chỉ, công ty, nhà thầu"
            />
          </label>
          {selects.map((s) => (
            <label key={s.key} className="ex-seg">
              <span>{s.label}</span>
              <select value={s.value} onChange={(e) => s.onChange(e.target.value)} aria-label={s.label}>
                {s.options.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </label>
          ))}
          <button className="ex-go" type="submit" aria-label="Tìm địa chỉ">
            <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
              <path d="M10.5 3a7.5 7.5 0 0 1 5.9 12.1l4.2 4.2-1.4 1.4-4.2-4.2A7.5 7.5 0 1 1 10.5 3zm0 2a5.5 5.5 0 1 0 0 11 5.5 5.5 0 0 0 0-11z" fill="currentColor" />
            </svg>
          </button>
        </form>
        {filterPanel && (
          <div className="ex-filter" ref={pop$}>
            <button type="button" className={`ex-filter-btn ${filterCount ? 'on' : ''}`} onClick={() => setOpen(!open)} aria-expanded={open}>
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
                <path d="M3 6h18v2H3zm4 5h10v2H7zm3 5h4v2h-4z" fill="currentColor" />
              </svg>
              Bộ lọc{filterCount ? <b>{filterCount}</b> : null}
            </button>
            {open && <div className="ex-filter-pop">{filterPanel}</div>}
          </div>
        )}
      </div>

      <div className="ex-body">
        <section className="ex-list">
          {layered && (
            <div className="ex-tabs" role="tablist">
              <button type="button" role="tab" aria-selected={tab === 'projects'} className={tab === 'projects' ? 'on' : ''} onClick={() => setTab('projects')}>
                Công trình <span>{rows.length}</span>
              </button>
              <button type="button" role="tab" aria-selected={tab === 'ctv'} className={tab === 'ctv' ? 'on' : ''} onClick={() => setTab('ctv')}>
                Cộng tác viên <span>{ctvs.length}</span>
              </button>
              <button type="button" role="tab" aria-selected={tab === 'cus'} className={tab === 'cus' ? 'on' : ''} onClick={() => setTab('cus')}>
                Khách hàng <span>{cus.length}</span>
              </button>
            </div>
          )}
          {tab === 'cus' && layered ? (
            <CustomerList rows={cus} all={allCus} owner={owner} setOwner={setOwner} onPick={pickCus} active={active} onHover={setActive} />
          ) : tab === 'ctv' && layered ? (
            <>
              <p className="muted ex-ctv-cap">
                {ctvs.length} cộng tác viên · {ctvs.filter((c) => c.type !== 'sales').length} cập nhật, {ctvs.filter((c) => c.type === 'sales').length} kinh doanh · bấm để xem trên bản
                đồ
              </p>
              <CtvList ctvs={ctvs} onPick={pickCtv} active={active} onHover={setActive} />
            </>
          ) : (
            <>
          <div className="ex-head">
            <h2>
              {rows.length} công trình {busy && <span className="muted ex-busy">· đang tải…</span>}
              {near && (
                <span className="ex-near">
                  trong {distText(near.radius)} quanh <b>{near.name.split(',')[0]}</b>
                  <button type="button" onClick={() => search.current?.clear()}>
                    Bỏ lọc vị trí ✕
                  </button>
                </span>
              )}
            </h2>
            <span className="ex-legend">
              <span className="ex-key" title={`CTV đã chụp trong ${staleDays} ngày`}>
                <PinMark /> Chưa tới lúc chụp
              </span>
              <span className="ex-key" title={`CTV chụp đã quá ${staleDays} ngày`}>
                <PinMark kind="stale" /> Cần chụp
              </span>
              {dz.length > 0 && (
                <span className="ex-key" title="Công trình nhà thầu đăng trên dezon.vn, chưa CTV nào chụp">
                  <PinMark kind="dz" /> Dezon ({dz.length})
                </span>
              )}
            </span>
          </div>
          {rows.length ? (
            <div className="ex-grid">
              {rows.slice(0, shown).map((it) => (
                <SiteCard
                  key={it.key}
                  p={it.p}
                  {...(it.d ? dezonCard(it.d, { prov: provName[it.d.province] }) : card(it.p))}
                  on={active === it.key}
                  onHover={setActive}
                  onLocate={() => setFocus({ id: it.key, n: (focus?.n || 0) + 1 })}
                  dist={dist ? distText(dist.get(it.key)) : null}
                />
              ))}
            </div>
          ) : (
            <p className="muted ex-empty">{near ? 'Chưa có công trình nào quanh vị trí này. Thử tăng bán kính trên bản đồ.' : 'Không có công trình nào theo bộ lọc.'}</p>
          )}
          {rows.length > shown && (
            <div className="ex-more">
              <button type="button" className="btn btn-secondary" onClick={() => setMore({ key: fitKey, n: shown + PAGE })}>
                Xem thêm {Math.min(PAGE, rows.length - shown)} công trình
              </button>
            </div>
          )}
          {footer}
            </>
          )}
        </section>
        <aside className="ex-map">
          <SiteMap
            points={points}
            provinces={provinces}
            selected={selectedProvince}
            onProvince={onProvince}
            fitKey={fitKey}
            pop={pop}
            staleDays={staleDays}
            active={active}
            onHover={setActive}
            focus={focus}
            fresh={fresh}
            search={search}
            onPlace={setNear}
            dezon={dz}
            ctvPins={layered ? ctvs : undefined}
            ctvPop={ctvTip}
            customers={layered ? cus : undefined}
            cusPop={(k) => cusTip(k, { edit: canEditCustomer })}
            overlay={layered ? <LayerPanel layers={layers} set={setLayers} counts={counts} /> : null}
          />
        </aside>
      </div>
    </div>
  )
}
