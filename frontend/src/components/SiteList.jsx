// Danh sách công trình cạnh bản đồ: mới chụp lại gần nhất trước; chấm màu theo lần chụp (vàng nhạt / xanh lá cần cập nhật).
// Bấm 1 dòng: bản đồ bay tới chấm đó và mở popup. Mỗi dòng có link Google Maps.
import { useState } from 'react'
import { AGE_FRESH, AGE_FRESH_LINE, AGE_STALE, gmapUrl, hsId, isStale, vnDate } from '@/utils/format'

const PAGE = 120

/** points: [{id, lat, lng, address, last_at|at, ...}]; meta(p): dòng phụ; badge(p): nhãn thêm; onPick(p); active: id đang chọn. */
export default function SiteList({ points, staleDays = 30, meta, badge, onPick, active }) {
  const [shown, setShown] = useState(PAGE)
  const rows = [...points].sort((a, b) => new Date(b.last_at || b.at) - new Date(a.last_at || a.at))
  if (!rows.length) return <p className="muted site-list-empty">Không có công trình nào theo bộ lọc.</p>
  return (
    <div className="site-list" role="list">
      {rows.slice(0, shown).map((p) => {
        const stale = isStale(p.last_at || p.at, staleDays)
        return (
          <div
            key={p.id}
            role="listitem"
            className={`site-row ${active === p.id ? 'on' : ''}`}
            onClick={() => onPick?.(p)}
            onKeyDown={(e) => e.key === 'Enter' && onPick?.(p)}
            tabIndex={0}
          >
            <i
              className="site-dot"
              style={{ background: stale ? AGE_STALE : AGE_FRESH, boxShadow: stale ? 'none' : `0 0 0 1px ${AGE_FRESH_LINE}` }}
              title={stale ? 'Cần cập nhật công trình' : `Chụp chưa quá ${staleDays} ngày`}
            />
            <div className="site-main">
              <div className="site-title">
                <span className="mono site-id">{hsId(p.id)}</span>
                {badge?.(p)}
                <b>{p.address || '(chưa có địa chỉ)'}</b>
              </div>
              {meta && <div className="site-meta">{meta(p)}</div>}
              <div className="site-foot">
                <span className={stale ? 'text-ok' : 'muted'}>
                  {stale ? 'Cần cập nhật · ' : ''}chụp {vnDate(p.last_at || p.at)}
                </span>
                <a href={gmapUrl(p.lat, p.lng)} target="_blank" rel="noreferrer" onClick={(e) => e.stopPropagation()}>
                  Google Maps ↗
                </a>
              </div>
            </div>
          </div>
        )
      })}
      {rows.length > shown && (
        <button type="button" className="btn btn-ghost site-more" onClick={() => setShown(shown + PAGE)}>
          Xem thêm {Math.min(PAGE, rows.length - shown)} công trình ({rows.length - shown} còn lại)
        </button>
      )}
    </div>
  )
}
