// Lớp bản đồ (layout 21/11/2026): bật / tắt từng loại ghim công trình và cộng tác viên.
//   Công trình: Chưa tới lúc chụp (vàng) · Cần chụp (xanh lá) · Dezon (xanh dương)
//   Cộng tác viên: CTV cập nhật (hồng) · CTV kinh doanh (đỏ); trạng thái: Tích cực (ghim đặc) · Đã đăng ký (ghim rỗng)
import { useState } from 'react'
import { initials } from '@/utils/format'
import { CUSTOMER_KINDS, LAYER_META, ctvKey, ctvStatusLabel, ctvTypeLabel } from '@/utils/mapLayers'

function Switch({ on, onClick, label }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} className={`lp-switch ${on ? 'on' : ''}`} onClick={onClick}>
      <i />
    </button>
  )
}

function Row({ k, on, count, round, square, onToggle }) {
  const m = LAYER_META[k]
  return (
    <label className={`lp-row ${on ? '' : 'off'}`}>
      <input type="checkbox" checked={on} onChange={onToggle} />
      <span className={`lp-dot ${round ? 'round' : square ? 'square' : ''}`} style={{ '--c': m.color }} />
      <span className="lp-name">{m.label}</span>
      <span className="lp-n">{count}</span>
    </label>
  )
}

/** counts: { pending, shoot, dezon, ctvUpdate, ctvSales, ctvActive, ctvReg } */
export function LayerPanel({ layers: L, set, counts }) {
  const [open, setOpen] = useState(true)
  const P = ['pending', 'shoot', 'dezon']
  const C = ['ctvUpdate', 'ctvSales']
  const pOn = P.some((k) => L[k])
  const cOn = C.some((k) => L[k])
  const nOn = Object.values(L).filter(Boolean).length
  return (
    <div className="lp" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} onWheel={(e) => e.stopPropagation()}>
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
              <Row key={k} k={k} round on={L[k]} count={counts[k]} onToggle={() => set({ [k]: !L[k] })} />
            ))}
            <div className="lp-chips">
              <span>Trạng thái CTV</span>
              <button type="button" className={`lp-chip ${L.ctvActive ? 'on' : ''}`} aria-pressed={L.ctvActive} onClick={() => set({ ctvActive: !L.ctvActive })}>
                <i className="solid" /> Tích cực <em>{counts.ctvActive}</em>
              </button>
              <button type="button" className={`lp-chip ${L.ctvReg ? 'on' : ''}`} aria-pressed={L.ctvReg} onClick={() => set({ ctvReg: !L.ctvReg })}>
                <i /> Đã đăng ký <em>{counts.ctvReg}</em>
              </button>
            </div>
          </div>
          <div className="lp-group">
            <div className="lp-ghead">
              <b>Khách hàng</b>
              <span className="lp-n">{counts.customer}</span>
              <Switch on={L.customer} label="Bật / tắt khách hàng" onClick={() => set({ customer: !L.customer })} />
            </div>
            <Row k="customer" square on={L.customer} count={counts.customer} onToggle={() => set({ customer: !L.customer })} />
          </div>
        </div>
      )}
    </div>
  )
}

/** Danh sách khách hàng (tab Khách hàng) + ô lọc Phụ trách. onPick(k): bay tới ghim khách. */
export function CustomerList({ rows, all, owner, setOwner, onPick, active, onHover }) {
  // người phụ trách có khách (đếm theo toàn bộ khách, không theo ô tìm)
  const owners = [...new Map(all.filter((k) => k.ctv_id).map((k) => [k.ctv_id, k.ctv_name])).entries()]
    .map(([id, name]) => ({ id, name, n: all.filter((k) => k.ctv_id === id).length }))
    .sort((a, b) => a.name.localeCompare(b.name, 'vi'))
  const by = (kind) => rows.filter((k) => k.kind === kind).length
  return (
    <>
      <div className="cus-head">
        <p className="muted">
          {rows.length} khách hàng · {Object.entries(CUSTOMER_KINDS).map(([k, l]) => `${by(k)} ${l.toLowerCase()}`).join(', ')}
        </p>
        <label className={`cus-owner ${owner ? 'on' : ''}`}>
          <span>Phụ trách</span>
          <b>{owner ? owners.find((o) => String(o.id) === owner)?.name || 'Tất cả' : 'Tất cả'}</b>
          <select value={owner} onChange={(e) => setOwner(e.target.value)} aria-label="Lọc theo người phụ trách">
            <option value="">Tất cả</option>
            {owners.map((o) => (
              <option key={o.id} value={String(o.id)}>
                {o.name} ({o.n})
              </option>
            ))}
          </select>
        </label>
      </div>
      {rows.length ? (
        <ul className="ctv-list">
          {rows.map((k) => (
            <li key={k.id}>
              <button
                type="button"
                className={`ctv-item ${active === `kh${k.id}` ? 'on' : ''}`}
                onClick={() => onPick(k)}
                onMouseEnter={() => onHover?.(`kh${k.id}`)}
                onMouseLeave={() => onHover?.(null)}
              >
                <span className="cus-av">KH</span>
                <span className="ctv-main">
                  <span className="ctv-line">
                    <b>{k.name}</b>
                    <span className="cus-tag">{CUSTOMER_KINDS[k.kind] || k.kind}</span>
                  </span>
                  <span className="ctv-addr">
                    {k.address || 'Chưa có địa chỉ'}
                    {k.lat == null ? ' · chưa có vị trí trên bản đồ' : ''}
                  </span>
                </span>
                <span className="cus-owner-col">
                  <small>Phụ trách</small>
                  <b>{k.ctv_name || '—'}</b>
                </span>
              </button>
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
