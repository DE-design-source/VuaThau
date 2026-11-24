// Khung trang Bản đồ kiểu Airbnb (admin + sale):
//   trên: thanh tìm dạng viên thuốc (Địa chỉ | các ô lọc | nút tìm tròn) + nút Bộ lọc;
//   trái: "N công trình" + lưới thẻ ảnh (3 cột trên màn rộng); phải: bản đồ bo góc đứng yên khi cuộn, nhãn mã hồ sơ.
import { useEffect, useMemo, useRef, useState } from 'react'
import SiteMap from './SiteMap'
import SiteCard from './SiteCard'
import { useNavigate } from 'react-router-dom'
import { ctvTip, cusTip, dezonCard } from '@/utils/siteTip'
import { distM, distText, hsId, isStale as stale } from '@/utils/format'
import { CtvList, CustomerList, FilterBar, LayerPanel, Pill } from './layers'
import { NO_CUS_FILTER, NO_P_FILTER, NO_V_FILTER, P_AGES, ctvKey, ctvProvinces, cusMatch, ctvVisible, facet, fold, pMatch, useLayers, vMatch } from '@/utils/mapLayers'

const PAGE = 24
const NONE = []
const SAME_SITE_M = 60 // công trình Dezon cách hồ sơ CTV dưới chừng này: coi là đã có CTV chụp, không hiện riêng

/** Ô tìm nhanh (q đã bỏ dấu): khớp chữ trong xs, hoặc từ 4 số trở lên khớp SĐT */
function qHit(q, xs, phone) {
  if (!q || fold(xs.join(' ')).includes(q)) return true
  const digits = q.replace(/\D/g, '')
  return digits.length >= 4 && (phone || '').replace(/\D/g, '').includes(digits)
}

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
export default function Explorer({ points: allPoints, provinces, staleDays = 30, selects = [], selectedProvince, onProvince, fitKey, pop, card, fresh, filterPanel, filterCount = 0, footer, busy, dezon = NONE, ctvPins, customers, ageColors = false }) {
  const [addr, setAddr] = useState('')
  const layered = Array.isArray(ctvPins)
  const [layers, setLayers] = useLayers()
  const [tab, setTab] = useState('projects')
  const navigate = useNavigate()
  // tìm nhanh trong danh sách (chỉ trang có lớp CTV): bỏ dấu, chữ thường
  const q = layered ? fold(addr.trim()) : ''
  // bộ lọc tab Công trình (góp ý 23-24/11/2026): tỉnh thành (ô của thanh tìm), giai đoạn, lần chụp, ngày chụp, CTV
  const [pf, setPf] = useState(NO_P_FILTER)
  // Danh sách tính sẵn (useMemo): rê chuột (đổi active) không vẽ lại ghim, thẻ đang mở trên bản đồ không tự tắt (góp ý 23/11/2026)
  const points = useMemo(
    () =>
      allPoints.filter(
        (p) =>
          (!layered || layers[stale(p.last_at || p.at, staleDays) ? 'shoot' : 'pending']) &&
          pMatch(p, pf, staleDays) &&
          (!q || fold([p.address, p.contractor, p.ctv_name, p.stage, hsId(p.id)].join(' ')).includes(q)),
      ),
    [allPoints, layered, layers, staleDays, pf, q],
  )
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
  const dz = useMemo(
    () =>
      // lọc theo CTV / giai đoạn / lần chụp thì bỏ công trình Dezon (chưa CTV nào chụp, không có giai đoạn)
      (layered && !layers.dezon) || pf.ctv || pf.stage || pf.shot
        ? NONE
        : dzAll.filter((d) => pMatch({ at: d.posted_at }, pf) && (!q || fold([d.title, d.contractor, d.address].join(' ')).includes(q))),
    [dzAll, layered, layers, pf, q],
  )
  // cộng tác viên theo lớp đang bật + ô lọc tab CTV + ô tìm
  const [vf, setVf] = useState(NO_V_FILTER)
  const ctvProv = useMemo(() => (layered ? ctvProvinces(ctvPins, provinces, allPoints) : new Map()), [layered, ctvPins, provinces, allPoints])
  const ctvs = useMemo(
    () => (layered ? ctvPins.filter((c) => ctvVisible(c, layers) && vMatch(c, vf, ctvProv.get(c.id)) && qHit(q, [c.name, c.address, c.kind], c.phone)) : NONE),
    [layered, ctvPins, layers, vf, q, ctvProv],
  )
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
  const [cf, setCf] = useState(NO_CUS_FILTER) // bộ lọc tab Khách hàng
  const allCus = layered ? customers || NONE : NONE
  const cus = useMemo(
    () => (layered && layers.customer ? allCus.filter((k) => cusMatch(k, cf) && qHit(q, [k.name, k.address, k.area, k.owner, k.kind], k.phone)) : NONE),
    [layered, layers, allCus, cf, q],
  )
  if (counts) counts.customer = allCus.length
  const pickCus = (k) => {
    if (k.lat == null) {
      window.open(`https://dezon.tech/kinh-doanh?lead=${encodeURIComponent(k.ext_id)}`, '_blank', 'noopener') // chưa có vị trí: mở bên Dezon Pro
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
  const stageOpts = useMemo(() => facet(allPoints, (p) => (p.stage || '').trim(), 'Chưa rõ giai đoạn'), [allPoints])
  const ctvOpts = useMemo(
    () =>
      [...new Map(allPoints.filter((p) => p.ctv_id).map((p) => [p.ctv_id, p.ctv_name])).entries()]
        .map(([id, name]) => ({ value: String(id), label: `${name || 'CTV ' + id} (${allPoints.filter((p) => p.ctv_id === id).length})` }))
        .sort((a, b) => a.label.localeCompare(b.label, 'vi')),
    [allPoints],
  )
  // ô Tỉnh thành tab Công trình: dùng chung ô của thanh tìm (lọc ở máy chủ), 2 chỗ luôn khớp nhau
  const provSel = selects.find((s) => s.key === 'province')
  // Lần chụp: như chú thích ghim (vàng / xanh lá)
  const shotOpts = useMemo(() => {
    const n = allPoints.filter((p) => stale(p.last_at || p.at, staleDays)).length
    return [
      { value: 'pending', label: `Chưa tới lúc chụp (${allPoints.length - n})` },
      { value: 'shoot', label: `Cần chụp (${n})` },
    ]
  }, [allPoints, staleDays])
  // Tỉnh thành tab CTV: theo địa chỉ CTV / nơi CTV chụp nhiều nhất (ctvProvinces)
  const ctvProvOpts = useMemo(() => {
    const n = new Map()
    for (const c of layered ? ctvPins : NONE) n.set(ctvProv.get(c.id) || '', (n.get(ctvProv.get(c.id) || '') || 0) + 1)
    const nm = Object.fromEntries((provinces || []).map((p) => [p.code, p.name]))
    return [...n.entries()]
      .map(([code, k]) => ({ value: code || '-', label: `${code ? nm[code] || code : 'Chưa rõ tỉnh'} (${k})`, none: !code }))
      .sort((a, b) => a.none - b.none || a.label.localeCompare(b.label, 'vi'))
      .map(({ value, label }) => ({ value, label }))
  }, [layered, ctvPins, ctvProv, provinces])

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
            <CustomerList rows={cus} all={allCus} filter={cf} setFilter={setCf} onPick={pickCus} active={active} onHover={setActive} />
          ) : tab === 'ctv' && layered ? (
            <>
              <FilterBar any={Object.entries(vf).some(([k, v]) => v !== NO_V_FILTER[k])} onClear={() => setVf(NO_V_FILTER)}>
                <Pill label="Tỉnh thành" value={vf.prov} options={ctvProvOpts} onChange={(v) => setVf({ ...vf, prov: v })} />
                <Pill
                  label="Loại"
                  value={vf.type}
                  options={[
                    { value: 'update', label: 'CTV cập nhật' },
                    { value: 'sales', label: 'CTV kinh doanh' },
                  ]}
                  onChange={(v) => setVf({ ...vf, type: v })}
                />
                <Pill
                  label="Trạng thái"
                  value={vf.status}
                  options={[
                    { value: 'active', label: 'Tích cực' },
                    { value: 'registered', label: 'Đã đăng ký' },
                  ]}
                  onChange={(v) => setVf({ ...vf, status: v })}
                />
                <Pill
                  label="Công trình"
                  value={vf.has}
                  options={[
                    { value: 'yes', label: 'Đã có' },
                    { value: 'no', label: 'Chưa có' },
                  ]}
                  onChange={(v) => setVf({ ...vf, has: v })}
                />
                <Pill
                  label="Gửi gần nhất"
                  value={vf.last}
                  options={[
                    { value: '7d', label: '7 ngày qua' },
                    { value: '30d', label: '30 ngày qua' },
                    { value: 'none', label: 'Chưa gửi' },
                  ]}
                  onChange={(v) => setVf({ ...vf, last: v })}
                />
              </FilterBar>
              {/* số CTV đã có trên tab: ở đây chỉ chia theo loại */}
              <p className="muted ex-sum">
                {ctvs.filter((c) => c.type !== 'sales').length} CTV cập nhật · {ctvs.filter((c) => c.type === 'sales').length} CTV kinh doanh ·{' '}
                {ctvs.filter((c) => c.status === 'active').length} tích cực
              </p>
              <CtvList ctvs={ctvs} onPick={pickCtv} active={active} onHover={setActive} />
            </>
          ) : (
            <>
          {/* góp ý 23-24/11/2026: bỏ dòng "N công trình" (đã có số trên tab), thay bằng hàng ô lọc như tab Khách hàng */}
          <FilterBar
            any={!!provSel?.value || Object.entries(pf).some(([k, v]) => v !== NO_P_FILTER[k])}
            onClear={() => {
              setPf(NO_P_FILTER)
              if (provSel?.value) provSel.onChange('')
            }}
          >
            {provSel && <Pill label="Tỉnh thành" value={provSel.value} options={provSel.options.filter((o) => o.value)} onChange={provSel.onChange} />}
            <Pill label="Giai đoạn" value={pf.stage} options={stageOpts} onChange={(v) => setPf({ ...pf, stage: v })} />
            <Pill label="Lần chụp" value={pf.shot} options={shotOpts} onChange={(v) => setPf({ ...pf, shot: v })} />
            <Pill label="Ngày chụp" value={pf.age} options={P_AGES} onChange={(v) => setPf({ ...pf, age: v })} />
            {layered && <Pill label="CTV" value={pf.ctv} options={ctvOpts} onChange={(v) => setPf({ ...pf, ctv: v })} />}
          </FilterBar>
          <div className="ex-head">
            <h2 className="ex-head-sub">
              {!layered && `${rows.length} công trình `}
              {busy && <span className="muted ex-busy">đang tải…</span>}
              {near && (
                <span className="ex-near">
                  trong {distText(near.radius)} quanh <b>{near.name.split(',')[0]}</b>
                  <button type="button" onClick={() => search.current?.clear()}>
                    Bỏ lọc vị trí ✕
                  </button>
                </span>
              )}
            </h2>
            {ageColors ? (
              <span className="ex-legend">
                <span className="ex-key">
                  <PinMark /> Trong vòng {staleDays} ngày
                </span>
                <span className="ex-key">
                  <PinMark kind="stale" /> Quá {staleDays} ngày
                </span>
                <span className="ex-key">
                  <PinMark kind="dz" /> Từ 2 tháng trước
                </span>
              </span>
            ) : (
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
            )}
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
            ageColors={ageColors}
            ctvPins={layered ? ctvs : undefined}
            ctvPop={ctvTip}
            customers={layered ? cus : undefined}
            cusPop={cusTip}
            overlay={layered ? <LayerPanel layers={layers} set={setLayers} counts={counts} /> : null}
          />
        </aside>
      </div>
    </div>
  )
}
