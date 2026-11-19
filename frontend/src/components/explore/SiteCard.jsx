// Thẻ công trình (thiết kế 19/11): ảnh 4:3 bo góc (ảnh cỡ thẻ 640px, chỉ tải khi cuộn tới), nhãn giai đoạn nổi góc trái,
// nút ghim góc phải (bay tới trên bản đồ); bên dưới: số nhà + đường, khu vực, nhà thầu; dòng cuối: CTV và ngày chụp.
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { getBlob } from '@/api/axiosClient'

// Ảnh đã tải (id ảnh -> URL blob), dùng lại khi lọc / cuộn lại
const cache = new Map()

function CardImage({ id }) {
  const box = useRef(null)
  const [src, setSrc] = useState(() => (id && cache.get(id)?.url) || null)
  useEffect(() => {
    if (!id || src) return
    const node = box.current
    let dead = false
    const load = () => {
      if (!cache.has(id)) {
        const entry = { url: null }
        entry.p = getBlob(`/photos/${id}`, { size: 'card' }).then(
          (b) => (entry.url = URL.createObjectURL(b)),
          () => null,
        )
        cache.set(id, entry)
      }
      cache.get(id).p.then((u) => !dead && u && setSrc(u))
    }
    const io = new IntersectionObserver(
      (es) => {
        if (es.some((e) => e.isIntersecting)) {
          io.disconnect()
          load()
        }
      },
      { rootMargin: '300px' },
    )
    io.observe(node)
    return () => {
      dead = true
      io.disconnect()
    }
  }, [id, src])
  return <div ref={box} className="ex-img">{src ? <img src={src} alt="" /> : <div className="ex-img-ph">{id ? '' : 'Chưa có ảnh'}</div>}</div>
}

/**
 * p: công trình; to: link trang chi tiết; tag: nhãn trên ảnh (giai đoạn); street / area: địa chỉ tách 2 dòng;
 * contractor; ctv: tên CTV; date: ngày chụp lần cuối; stale: quá hạn cần cập nhật; dist: khoảng cách (lọc quanh vị trí).
 * on: đang rê chuột (từ bản đồ); onHover(id|null); onLocate(): bay tới trên bản đồ.
 * Khác: xem SiteCard bên dưới (id, img, href, dz, source cho thẻ Dezon).
 */

// Link trong app (to) hoặc ra trang ngoài (href, mở tab mới: công trình Dezon)
function Go({ to, href, className, label, children }) {
  return href ? (
    <a href={href} target="_blank" rel="noreferrer" className={className} aria-label={label}>
      {children}
    </a>
  ) : (
    <Link to={to} className={className} aria-label={label}>
      {children}
    </Link>
  )
}

/** id: khoá thẻ (trùng khoá ghim trên bản đồ); img: ảnh công khai (Dezon) thay cho ảnh hồ sơ cần đăng nhập; dz: thẻ Dezon. */
export default function SiteCard({ p, id = p.id, to, href, img, dz, tag, street, area, contractor, ctv, source, date, stale, dist, on, onHover, onLocate }) {
  const ini = (ctv || '').trim().split(/\s+/).pop()?.[0]?.toUpperCase()
  return (
    <article className={`ex-card ${on ? 'on' : ''}`} onMouseEnter={() => onHover?.(id)} onMouseLeave={() => onHover?.(null)}>
      <div className="ex-media">
        <Go to={to} href={href} label={street}>
          {img ? (
            <div className="ex-img">
              <img src={img} alt="" loading="lazy" referrerPolicy="no-referrer" onError={(e) => (e.currentTarget.style.visibility = 'hidden')} />
            </div>
          ) : (
            <CardImage id={p.cover_id} />
          )}
        </Go>
        {tag && <span className={`ex-badge ${dz ? 'dz' : ''}`}>{tag}</span>}
        <button type="button" className="ex-gmap" onClick={onLocate} title="Xem trên bản đồ" aria-label="Xem trên bản đồ">
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <path d="M8 1a5 5 0 0 0-5 5c0 3.6 5 9 5 9s5-5.4 5-9a5 5 0 0 0-5-5zm0 7a2 2 0 1 1 0-4 2 2 0 0 1 0 4z" fill="currentColor" />
          </svg>
        </button>
      </div>
      <Go to={to} href={href} className="ex-info">
        <div className="ex-title">{street}</div>
        {(area || dist) && (
          <div className="ex-line">
            {area}
            {dist && <b className="ex-dist"> · cách {dist}</b>}
          </div>
        )}
        {contractor && <div className="ex-line ex-ct">{contractor}</div>}
      </Go>
      <div className="ex-foot">
        {source ? (
          <span className="ex-ctv">{source}</span>
        ) : ctv ? (
          <span className="ex-ctv">
            <i>{ini || '?'}</i>
            <span>
              CTV <b>{ctv}</b>
            </span>
          </span>
        ) : (
          <span />
        )}
        {date && (
          <span className={stale ? 'ex-stale' : ''} title={stale ? 'Quá hạn, cần chụp cập nhật' : 'Chụp lần cuối'}>
            {date}
          </span>
        )}
      </div>
    </article>
  )
}
