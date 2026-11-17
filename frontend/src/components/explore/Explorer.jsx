// Khung trang Bản đồ kiểu Airbnb (admin + sale):
//   trên: thanh tìm dạng viên thuốc (Địa chỉ | các ô lọc | nút tìm tròn) + nút Bộ lọc;
//   trái: "N công trình" + lưới thẻ ảnh 2 cột; phải: bản đồ bo góc đứng yên khi cuộn, nhãn viên thuốc.
import { useEffect, useRef, useState } from 'react'
import SiteMap from './SiteMap'
import SiteCard from './SiteCard'

const PAGE = 24

/** Ghim nhỏ trong chú thích (cùng hình với ghim trên bản đồ). */
function PinMark({ stale }) {
  return (
    <svg className={`pin-mark ${stale ? 'stale' : ''}`} viewBox="0 0 32 42" width="14" height="19" aria-hidden="true">
      <path d="M16 1.5C8.3 1.5 2 7.7 2 15.4 2 25.8 16 40.5 16 40.5S30 25.8 30 15.4C30 7.7 23.7 1.5 16 1.5z" fill="currentColor" />
      <circle cx="16" cy="15.4" r="6.2" fill="#fff" />
    </svg>
  )
}

/**
 * points; provinces; staleDays
 * selects: [{ key, label, value, options: [{ value, label }], onChange(v) }] ô lọc trong thanh tìm
 * selectedProvince, onProvince(code): bấm tỉnh trên bản đồ; fitKey: đổi khi đổi bộ lọc (bản đồ canh lại)
 * pop(p): HTML thẻ khi bấm nhãn; card(p): props cho SiteCard (to, badge, title, right, lines, when, chips)
 * fresh: Set id mới (thời gian thực); filterPanel: nội dung nút Bộ lọc; filterCount: số bộ lọc đang bật
 * footer: phần cuối danh sách (số liệu...); busy: đang tải
 */
export default function Explorer({ points, provinces, staleDays = 30, selects = [], selectedProvince, onProvince, fitKey, pop, card, fresh, filterPanel, filterCount = 0, footer, busy }) {
  const [addr, setAddr] = useState('')
  const search = useRef(null) // MapSearch: search.current.find(chữ)
  const [active, setActive] = useState(null)
  const [focus, setFocus] = useState(null)
  // số thẻ đang hiện, gắn với bộ lọc: đổi bộ lọc thì về trang đầu
  const [more, setMore] = useState({ key: fitKey, n: PAGE })
  const shown = more.key === fitKey ? more.n : PAGE
  const [open, setOpen] = useState(false)
  const pop$ = useRef(null)

  // Bấm ra ngoài: đóng Bộ lọc
  useEffect(() => {
    if (!open) return
    const close = (e) => !pop$.current?.contains(e.target) && setOpen(false)
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  const rows = [...points].sort((a, b) => new Date(b.last_at || b.at) - new Date(a.last_at || a.at))

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
            <input value={addr} onChange={(e) => setAddr(e.target.value)} placeholder="Số nhà, đường, phường…" aria-label="Tìm địa chỉ" />
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
          <div className="ex-head">
            <h2>
              {rows.length} công trình {busy && <span className="muted ex-busy">· đang tải…</span>}
            </h2>
            <span className="ex-legend">
              <PinMark /> chưa quá {staleDays} ngày
              <PinMark stale /> cần cập nhật
            </span>
          </div>
          {rows.length ? (
            <div className="ex-grid">
              {rows.slice(0, shown).map((p) => (
                <SiteCard
                  key={p.id}
                  p={p}
                  {...card(p)}
                  on={active === p.id}
                  onHover={setActive}
                  onLocate={() => setFocus({ id: p.id, n: (focus?.n || 0) + 1 })}
                />
              ))}
            </div>
          ) : (
            <p className="muted ex-empty">Không có công trình nào theo bộ lọc.</p>
          )}
          {rows.length > shown && (
            <div className="ex-more">
              <button type="button" className="btn btn-secondary" onClick={() => setMore({ key: fitKey, n: shown + PAGE })}>
                Xem thêm {Math.min(PAGE, rows.length - shown)} công trình
              </button>
            </div>
          )}
          {footer}
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
          />
        </aside>
      </div>
    </div>
  )
}
