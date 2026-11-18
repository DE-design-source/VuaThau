// Thẻ công trình kiểu Airbnb: ảnh lớn bo góc (ảnh cỡ thẻ 640px, chỉ tải khi cuộn tới), nhãn nổi trên ảnh,
// nút Google Maps góc ảnh, bên dưới: tiêu đề + các dòng thông tin + nhãn nhỏ.
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { getBlob } from '@/api/axiosClient'
import { gmapUrl } from '@/utils/format'

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
 * p: công trình; to: link trang chi tiết; badge: chữ nhãn nổi trên ảnh (vd "VIP · chụp đầu tiên");
 * title / right: dòng tiêu đề và chữ bên phải; lines: các dòng chữ xám; when: dòng ngày chụp; chips: nhãn nhỏ cuối thẻ.
 * on: đang rê chuột (từ bản đồ); onHover(id|null); onLocate(): bay tới trên bản đồ.
 */
export default function SiteCard({ p, to, badge, title, right, lines = [], when, chips = [], on, onHover, onLocate, dist }) {
  return (
    <article className={`ex-card ${on ? 'on' : ''}`} onMouseEnter={() => onHover?.(p.id)} onMouseLeave={() => onHover?.(null)}>
      <div className="ex-media">
        <Link to={to} aria-label={title}>
          <CardImage id={p.cover_id} />
        </Link>
        {badge && <span className="ex-badge">{badge}</span>}
        <a className="ex-gmap" href={gmapUrl(p.lat, p.lng)} target="_blank" rel="noreferrer" title="Mở Google Maps" aria-label="Mở Google Maps">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path d="M12 2a7 7 0 0 0-7 7c0 5.2 7 13 7 13s7-7.8 7-13a7 7 0 0 0-7-7zm0 9.5a2.5 2.5 0 1 1 0-5 2.5 2.5 0 0 1 0 5z" fill="currentColor" />
          </svg>
        </a>
      </div>
      <div className="ex-info">
        <div className="ex-row">
          <Link to={to} className="ex-title">
            {title}
          </Link>
          {right && <span className="ex-right">{right}</span>}
        </div>
        {lines.filter(Boolean).map((l, i) => (
          <div key={i} className="ex-line">
            {l}
          </div>
        ))}
        {when && <div className="ex-when">{when}</div>}
        <div className="ex-chips">
          {dist && <span className="ex-chip ex-chip-dist">Cách {dist}</span>}
          {chips.filter(Boolean).map((c, i) => (
            <span key={i} className="ex-chip">
              {c}
            </span>
          ))}
          <button type="button" className="ex-chip ex-chip-btn" onClick={onLocate}>
            Xem trên bản đồ
          </button>
        </div>
      </div>
    </article>
  )
}
