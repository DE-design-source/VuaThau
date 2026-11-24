// Lớp bản đồ (layout 21/11/2026): bật / tắt từng loại ghim công trình, cộng tác viên, khách hàng.
//   Công trình: Chưa tới lúc chụp (vàng) · Cần chụp (xanh lá) · Dezon (xanh dương)
//   Cộng tác viên (bộ icon 23/11/2026): CTV cam (đặc: tích cực, viền: đã đăng ký) · SALE tím (CTV kinh doanh)
//   Khách hàng: KH đỏ. Chú thích vẽ đúng ghim trên bản đồ (pins.js).
import { useState } from 'react'
import { initials } from '@/utils/format'
import { cusGmap } from '@/utils/siteTip'
import { CUS_SORTS, LAYER_META, NO_CUS_FILTER, facet, RATING_LABEL, ctvKey, ctvStatusLabel, ctvTypeLabel } from '@/utils/mapLayers'
import { MK, badgePin } from './pins'

function Switch({ on, onClick, label }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} className={`lp-switch ${on ? 'on' : ''}`} onClick={onClick}>
      <i />
    </button>
  )
}

/** Ghim nhỏ trong chú thích, giống hệt ghim trên bản đồ (SVG tĩnh, không có dữ liệu người dùng) */
function Ico({ html }) {
  return <span className="lp-ico" dangerouslySetInnerHTML={{ __html: html }} />
}
const ICO = {
  ctvUpdate: badgePin('CTV', MK.ctv, false, 16),
  ctvSales: badgePin('SALE', MK.sale, false, 16),
  ctvReg: badgePin('CTV', MK.ctv, true, 14),
  ctvActive: badgePin('CTV', MK.ctv, false, 14),
  customer: badgePin('KH', MK.kh, false, 16),
}

function Row({ k, on, count, onToggle }) {
  const m = LAYER_META[k]
  return (
    <label className={`lp-row ${on ? '' : 'off'}`}>
      <input type="checkbox" checked={on} onChange={onToggle} />
      {ICO[k] ? <Ico html={ICO[k]} /> : <span className="lp-dot" style={{ '--c': m.color }} />}
      <span className="lp-name">{m.label}</span>
      <span className="lp-n">{count}</span>
    </label>
  )
}

/** counts: { pending, shoot, dezon, ctvUpdate, ctvSales, ctvActive, ctvReg } */
export function LayerPanel({ layers: L, set, counts }) {
  // thu gọn sẵn (không che bản đồ, góp ý 22/11/2026); mở / gọn nhớ theo trình duyệt
  const [open, setOpenState] = useState(() => {
    try {
      return localStorage.getItem('vuathau.layerPanel') === 'open'
    } catch {
      return false
    }
  })
  const setOpen = (v) => {
    setOpenState(v)
    try {
      localStorage.setItem('vuathau.layerPanel', v ? 'open' : 'closed')
    } catch {
      /* không lưu được: chỉ mất ghi nhớ */
    }
  }
  const P = ['pending', 'shoot', 'dezon']
  const C = ['ctvUpdate', 'ctvSales']
  const pOn = P.some((k) => L[k])
  const cOn = C.some((k) => L[k])
  const nOn = Object.values(L).filter(Boolean).length
  return (
    <div className={`lp ${open ? '' : 'closed'}`} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} onWheel={(e) => e.stopPropagation()}>
      <button type="button" className="lp-head" onClick={() => setOpen(!open)} aria-expanded={open}>
        <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
          <path d="m12 3 9 5-9 5-9-5 9-5ZM3 13l9 5 9-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
        </svg>
        <span>Lớp bản đồ</span>
        <small>{open ? 'Thu gọn' : `${nOn}/8 bật`}</small>
      </button>
      {open && (
        <div className="lp-body">
          <div className="lp-group">
            <div className="lp-ghead">
              <b>Công trình</b>
              <span className="lp-n">{counts.pending + counts.shoot + counts.dezon}</span>
              <Switch on={pOn} label="Bật / tắt công trình" onClick={() => set(Object.fromEntries(P.map((k) => [k, !pOn])))} />
            </div>
            {P.map((k) => (
              <Row key={k} k={k} on={L[k]} count={counts[k]} onToggle={() => set({ [k]: !L[k] })} />
            ))}
          </div>
          <div className="lp-group">
            <div className="lp-ghead">
              <b>Cộng tác viên</b>
              <span className="lp-n">{counts.ctvUpdate + counts.ctvSales}</span>
              <Switch on={cOn} label="Bật / tắt cộng tác viên" onClick={() => set(Object.fromEntries(C.map((k) => [k, !cOn])))} />
            </div>
            {C.map((k) => (
              <Row key={k} k={k} on={L[k]} count={counts[k]} onToggle={() => set({ [k]: !L[k] })} />
            ))}
            <div className="lp-chips">
              <span>Trạng thái CTV</span>
              <button type="button" className={`lp-chip ${L.ctvActive ? 'on' : ''}`} aria-pressed={L.ctvActive} onClick={() => set({ ctvActive: !L.ctvActive })}>
                <Ico html={ICO.ctvActive} /> Tích cực <em>{counts.ctvActive}</em>
              </button>
              <button type="button" className={`lp-chip ${L.ctvReg ? 'on' : ''}`} aria-pressed={L.ctvReg} onClick={() => set({ ctvReg: !L.ctvReg })}>
                <Ico html={ICO.ctvReg} /> Đã đăng ký <em>{counts.ctvReg}</em>
              </button>
            </div>
          </div>
          <div className="lp-group">
            <div className="lp-ghead">
              <b>Khách hàng</b>
              <span className="lp-n">{counts.customer}</span>
              <Switch on={L.customer} label="Bật / tắt khách hàng" onClick={() => set({ customer: !L.customer })} />
            </div>
            <Row k="customer" on={L.customer} count={counts.customer} onToggle={() => set({ customer: !L.customer })} />
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * Hàng ô lọc dùng chung 3 tab Công trình | Cộng tác viên | Khách hàng (góp ý 24/11/2026: các ô cùng cao, cách đều,
 * 1 hàng gọn; màn hẹp thì xuống dòng; "Bỏ lọc" luôn nằm cuối hàng). any: đang lọc; onClear: bỏ mọi ô.
 */
export function FilterBar({ any, onClear, children }) {
  return (
    <div className="ex-filters">
      {children}
      <button type="button" className="ex-filters-clear" onClick={onClear} disabled={!any} aria-hidden={!any} tabIndex={any ? 0 : -1}>
        Bỏ lọc ✕
      </button>
    </div>
  )
}

/** Ô lọc dạng viên thuốc: nhãn + giá trị đang chọn, bấm mở danh sách (select trong suốt phủ lên). */
/** all: chữ của lựa chọn trống (mặc định "Tất cả"; ô Sắp xếp: thứ tự mặc định) */
export function Pill({ label, value, options, onChange, all = 'Tất cả' }) {
  const cur = options.find((o) => o.value === value)
  return (
    <label className={`cus-owner ${value ? 'on' : ''}`}>
      <span>{label}</span>
      <b>{value ? cur?.label?.replace(/ \(\d+\)$/, '') || value : all}</b>
      <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}>
        <option value="">{all}</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}

/** Danh sách khách hàng (tab Khách hàng, lấy từ Dezon Pro) + hàng ô lọc. onPick(k): bay tới ghim khách. */
export function CustomerList({ rows, all, filter: f, setFilter, onPick, active, onHover }) {
  const set = (patch) => setFilter({ ...f, ...patch })
  const owners = facet(all, (k) => k.owner, 'Chưa có').filter((o) => o.value !== '-')
  const provinces = facet(all, (k) => k.province, 'Chưa rõ tỉnh')
  const sources = facet(all, (k) => k.source, 'Chưa ghi nguồn')
  const ratings = [5, 4, 3, 2, 1, 0].map((r) => ({ value: String(r), label: `${r ? '★'.repeat(r) + ' ' : ''}${RATING_LABEL[r]} (${all.filter((k) => (k.rating || 0) === r).length})` }))
  const dates = [
    { value: 'week', label: 'Tuần này' },
    { value: '30d', label: '30 ngày qua' },
    { value: 'custom', label: 'Tuỳ chỉnh…' },
  ]
  const any = Object.entries(f).some(([k, v]) => v !== NO_CUS_FILTER[k])
  // đếm theo loại công trình (Nhà phố, Biệt thự, Căn hộ...), nhiều nhất trước
  const kinds = [...new Set(rows.map((k) => k.kind || 'Chưa xác định'))]
    .map((kd) => [kd, rows.filter((k) => (k.kind || 'Chưa xác định') === kd).length])
    .sort((a, b) => b[1] - a[1])
  if (!all.length) {
    return (
      <p className="muted ex-empty">
        Chưa có khách hàng. Khách hàng lấy tự động từ pipeline Kinh doanh của Dezon Pro (dezon.tech) mỗi 15 phút, sau khi máy chủ có khoá kết nối.
      </p>
    )
  }
  return (
    <>
      <FilterBar any={any} onClear={() => setFilter(NO_CUS_FILTER)}>
        <Pill label="Tỉnh thành" value={f.province} options={provinces} onChange={(v) => set({ province: v })} />
        <Pill label="Nguồn" value={f.source} options={sources} onChange={(v) => set({ source: v })} />
        <Pill label="Giao tiếp" value={f.rating} options={ratings} onChange={(v) => set({ rating: v })} />
        <Pill label="Ngày tạo" value={f.date} options={dates} onChange={(v) => set({ date: v, ...(v === 'custom' ? {} : { from: '', to: '' }) })} />
        <Pill label="Phụ trách" value={f.owner} options={owners} onChange={(v) => set({ owner: v })} />
        <Pill label="Sắp xếp" value={f.sort} options={CUS_SORTS} onChange={(v) => set({ sort: v })} all="Mặc định" />
      </FilterBar>
      {/* số khách đã có trên tab: ở đây chỉ chia theo loại công trình */}
      {kinds.length > 0 && <p className="muted ex-sum">{kinds.map(([kd, n]) => `${n} ${kd.toLowerCase()}`).join(' · ')}</p>}
      {f.date === 'custom' && (
        <div className="cus-filters">
          <span className="cus-dates">
            <span className="muted">Ngày tạo từ</span>
            <input className="input input-sm" type="date" value={f.from} max={f.to || undefined} onChange={(e) => set({ from: e.target.value })} aria-label="Từ ngày" />
            <span className="muted">→</span>
            <input className="input input-sm" type="date" value={f.to} min={f.from || undefined} onChange={(e) => set({ to: e.target.value })} aria-label="Đến ngày" />
          </span>
        </div>
      )}
      {rows.length ? (
        <ul className="ctv-list">
          {rows.map((k) => (
            <li key={k.id} className={`cus-li ${active === `kh${k.id}` ? 'on' : ''}`} onMouseEnter={() => onHover?.(`kh${k.id}`)} onMouseLeave={() => onHover?.(null)}>
              <button type="button" className="ctv-item" onClick={() => onPick(k)}>
                <span className="cus-av">KH</span>
                <span className="ctv-main">
                  <span className="ctv-line">
                    <b>{k.name}</b>
                    {k.kind && <span className="cus-tag">{k.kind}</span>}
                    {k.rating > 0 && (
                      <span className="cus-stars" title={`Trạng thái giao tiếp: ${RATING_LABEL[k.rating]}`}>
                        {'★'.repeat(k.rating)}
                      </span>
                    )}
                  </span>
                  <span className="ctv-addr">
                    {[k.address, k.area].filter(Boolean).join(', ') || 'Chưa có địa chỉ'}
                    {k.lat == null ? ' · chưa có vị trí trên bản đồ' : ''}
                  </span>
                </span>
                <span className="cus-owner-col">
                  <small>Phụ trách</small>
                  <b>{k.owner || '—'}</b>
                </span>
              </button>
              {/* chỉ khách đã có vị trí: mở Google Maps ở tab mới (link riêng, không lồng trong nút của dòng) */}
              {cusGmap(k) && (
                <a className="gmap-btn" href={cusGmap(k)} target="_blank" rel="noreferrer" title="Mở vị trí khách trên Google Maps">
                  Xem Google Map
                </a>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="muted ex-empty">Chưa có khách hàng nào theo lớp đang bật / ô tìm.</p>
      )}
    </>
  )
}

/** Danh sách cộng tác viên (tab Cộng tác viên). onPick(c): bay tới ghim CTV. */
export function CtvList({ ctvs, onPick, active, onHover }) {
  if (!ctvs.length) return <p className="muted ex-empty">Không có cộng tác viên nào theo lớp đang bật / ô tìm.</p>
  return (
    <ul className="ctv-list">
      {ctvs.map((c) => {
        const color = LAYER_META[ctvKey(c)].color
        const on = c.status === 'active'
        return (
          <li key={c.id}>
            <button
              type="button"
              className={`ctv-item ${active === `ctv${c.id}` ? 'on' : ''}`}
              onClick={() => onPick(c)}
              onMouseEnter={() => onHover?.(`ctv${c.id}`)}
              onMouseLeave={() => onHover?.(null)}
            >
              <span className={`ctv-av ${on ? 'solid' : ''}`} style={{ '--c': color }}>
                {initials(c.name)}
              </span>
              <span className="ctv-main">
                <span className="ctv-line">
                  <b>{c.name}</b>
                  <span className="ctv-tag" style={{ '--c': color }}>
                    {ctvTypeLabel(c)}
                  </span>
                  <span className="ctv-st">
                    <i className={on ? 'solid' : ''} style={{ '--c': color }} />
                    {ctvStatusLabel(c)}
                  </span>
                </span>
                <span className="ctv-addr">
                  {c.address || 'Chưa có địa chỉ'}
                  {c.lat == null ? ' · chưa rõ vị trí trên bản đồ' : c.approx ? ' · vị trí gần đúng' : ''}
                </span>
              </span>
              <span className="ctv-count">
                <b>{c.count}</b>
                <small>công trình</small>
              </span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}
