// Báo cáo thi công gửi chủ nhà (/bc/:code, thiết kế "Báo cáo khách hàng (link chủ nhà)" 24/11/2026):
// đầu trang công ty nhà thầu, cập nhật mới nhất (ảnh, hạng mục, lời nhà thầu, tiến độ), nhật ký trước đây,
// nút Gọi nhà thầu / Nhắn Zalo. Bấm ảnh: xem toàn màn hình (thiết kế "Xem ảnh toàn màn hình").
// Không cần đăng nhập, không menu quản lý; xem trên điện thoại là chính.
import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import publicApi from '@/api/publicApi'
import './publicReport.css'

const p2 = (n) => String(n).padStart(2, '0')
const WD = ['Chủ Nhật', 'Thứ Hai', 'Thứ Ba', 'Thứ Tư', 'Thứ Năm', 'Thứ Sáu', 'Thứ Bảy']
const CATS = ['Cốp pha', 'Cốt thép', 'Đổ bê tông', 'Xây tô']
const dayMonth = (t) => `${p2(t.getDate())}/${p2(t.getMonth() + 1)}`
const fullTime = (t) => `${WD[t.getDay()]}, ${dayMonth(t)}/${t.getFullYear()} · ${p2(t.getHours())}:${p2(t.getMinutes())}`

/** Ghi chú nhật ký "Cốp pha, Cốt thép. Lắp cốp pha cột tầng 2" -> hạng mục + phần lời nhà thầu. */
function splitNote(note = '') {
  const s = note.trim()
  const dot = s.indexOf('. ')
  const head = (dot > 0 ? s.slice(0, dot) : s).replace(/\.$/, '')
  const parts = head.split(', ')
  if (head && parts.every((x) => CATS.includes(x))) return { cats: parts, text: dot > 0 ? s.slice(dot + 2).trim() : '' }
  return { cats: [], text: s }
}

/** Chữ viết tắt công ty: bỏ "Công ty, TNHH, Xây dựng…" ("Công ty TNHH Xây dựng Smartcons" -> "SC"). */
function initials(name = '') {
  let n = ` ${name.toLowerCase()} `
  for (const w of ['công ty', 'cty', 'tnhh', 'cổ phần', 'cp', 'xây dựng', 'xd', 'thương mại', 'dịch vụ', 'kiến trúc', 'nội thất', 'thiết kế', 'mtv', 'và', '&'])
    n = n.split(` ${w} `).join(' ')
  const words = (n.trim() || name).split(/\s+/).filter(Boolean)
  if (!words.length) return 'NT'
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[words.length - 1][0]).toUpperCase()
}

/** Ảnh ghép kiểu thiết kế: hàng 1 hai ảnh (1 dọc + 1 ngang), hàng 2 ba ảnh; ảnh thừa hiện "+N ảnh" ở ô cuối. */
function Mosaic({ photos, onOpen, max = 5, small = false }) {
  if (!photos.length) return null
  const shown = photos.slice(0, max)
  const more = photos.length - shown.length
  const rows = small ? [shown] : [shown.slice(0, 2), shown.slice(2, 5)].filter((r) => r.length)
  let k = -1
  return (
    <div className={`rp-mosaic${small ? ' small' : ''}`}>
      {rows.map((row, ri) => (
        <div className="rp-mrow" key={ri}>
          {row.map((u, ci) => {
            k += 1
            const i = k
            const last = i === shown.length - 1 && more > 0
            // hàng đầu: ô trái dọc (3/4), ô phải ngang (4/3) như thiết kế; còn lại vuông dọc
            const wide = !small && ri === 0 && ci === 1
            return (
              <button type="button" key={u} className={wide ? 'wide' : ''} onClick={() => onOpen(i)} aria-label={`Xem ảnh ${i + 1}`}>
                <img src={`${u}?size=small`} alt="" loading="lazy" />
                {last && <span className="rp-more">+{more}{small ? '' : ' ảnh'}</span>}
              </button>
            )
          })}
        </div>
      ))}
    </div>
  )
}

/** Xem ảnh toàn màn hình: đếm ảnh, tải về, trước / sau (vuốt, phím mũi tên), GPS + giờ chụp, dải ảnh nhỏ. */
function Viewer({ set, start, onClose, title, company }) {
  const [i, setI] = useState(start)
  const touch = useRef(null)
  const n = set.photos.length
  const go = useCallback((d) => setI((x) => Math.min(n - 1, Math.max(0, x + d))), [n])
  useEffect(() => {
    const onKey = (e) => {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowRight') go(1)
      if (e.key === 'ArrowLeft') go(-1)
    }
    window.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    }
  }, [go, onClose])
  useEffect(() => {
    document.querySelector(`.rp-strip [data-i="${i}"]`)?.scrollIntoView({ block: 'nearest', inline: 'center', behavior: 'smooth' })
  }, [i])
  const t = new Date(set.takenAt)
  const url = set.photos[i]
  return (
    <div className="rp-viewer" role="dialog" aria-modal="true" aria-label="Xem ảnh">
      <header>
        <button type="button" className="rp-round" onClick={onClose} aria-label="Đóng">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>
        </button>
        <div className="rp-vtitle">
          <b>
            {i + 1} / {n}
          </b>
          <span>
            {title} · {dayMonth(t)}
          </span>
        </div>
        <a className="rp-round" href={url} download={`bao-cao-${dayMonth(t).replace('/', '-')}-${i + 1}.jpg`} aria-label="Tải ảnh về">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true"><path d="M12 4v12M7 11l5 5 5-5M4 20h16" /></svg>
        </a>
      </header>
      <div
        className="rp-stage"
        onTouchStart={(e) => (touch.current = e.touches[0].clientX)}
        onTouchEnd={(e) => {
          if (touch.current == null) return
          const dx = e.changedTouches[0].clientX - touch.current
          if (Math.abs(dx) > 40) go(dx < 0 ? 1 : -1)
          touch.current = null
        }}
      >
        <img src={url} alt={`Ảnh ${i + 1}/${n}`} />
        {i > 0 && (
          <button type="button" className="rp-round prev" onClick={() => go(-1)} aria-label="Ảnh trước">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" aria-hidden="true"><path d="M15 6l-6 6 6 6" /></svg>
          </button>
        )}
        {i < n - 1 && (
          <button type="button" className="rp-round next" onClick={() => go(1)} aria-label="Ảnh tiếp theo">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" aria-hidden="true"><path d="M9 6l6 6-6 6" /></svg>
          </button>
        )}
      </div>
      <div className="rp-vmeta">
        <div>
          {set.gps && (
            <span className="rp-gps">
              <i />
              GPS
            </span>
          )}
          <span className="num">{fullTime(t)}</span>
        </div>
        {(set.address || company) && <small>{[set.address, company].filter(Boolean).join(' · ')}</small>}
      </div>
      <div className="rp-strip">
        {set.photos.map((u, k) => (
          <button type="button" key={u} data-i={k} className={k === i ? 'on' : ''} onClick={() => setI(k)} aria-label={`Ảnh ${k + 1}`}>
            <img src={`${u}?size=small`} alt="" loading="lazy" />
          </button>
        ))}
      </div>
    </div>
  )
}

export default function PublicReportPage() {
  const { code } = useParams()
  const { data: r, err } = usePage(() => publicApi.report(code), [code])
  const [view, setView] = useState(null) // { set, start }
  const navigate = useNavigate()
  // vào từ trang khác trong web (hồ sơ năng lực -> dự án): quay lại trang đó; mở thẳng link (Zalo): sang hồ sơ năng lực
  const fromInside = useLocation().key !== 'default'
  const back = () => (fromInside ? navigate(-1) : navigate(r?.portfolio || '/'))

  useEffect(() => {
    if (r) document.title = `${r.project} · Báo cáo thi công`
  }, [r])

  if (!r) {
    return (
      <main className="rp">
        <div className="rp-empty">{err ? err : 'Đang mở báo cáo…'}</div>
      </main>
    )
  }
  const t = new Date(r.taken_at)
  const latest = { photos: r.photos || [], takenAt: r.taken_at, gps: r.gps, address: r.log_address || r.address }
  const { cats, text } = splitNote(r.note)
  const phone = (r.phone || '').replace(/\D/g, '')
  const done = r.status === 'hoan_thanh'
  return (
    <main className="rp">
      <header className="rp-top">
        {/* góp ý 25/11/2026: nút < về lại trang trước đó */}
        {(fromInside || r.portfolio) && (
          <button type="button" className="rp-back" onClick={back} aria-label={fromInside ? 'Quay lại trang trước' : 'Xem hồ sơ năng lực nhà thầu'} title={fromInside ? 'Quay lại' : 'Hồ sơ năng lực nhà thầu'}>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M15 5l-7 7 7 7" />
            </svg>
          </button>
        )}
        <span className="rp-logo">{initials(r.company)}</span>
        <div className="rp-co">
          <b>{r.company || 'Nhà thầu'}</b>
          {r.regions && (
            <span className="rp-area">
              <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M12 2a7 7 0 0 0-7 7c0 5.2 7 13 7 13s7-7.8 7-13a7 7 0 0 0-7-7zm0 9.5A2.5 2.5 0 1 1 12 6.5a2.5 2.5 0 0 1 0 5z" />
              </svg>
              {r.regions}
            </span>
          )}
          <small>Báo cáo thi công gửi quý khách hàng{r.verified ? ' · Vua Thầu đã xác minh' : ''}</small>
        </div>
      </header>

      <section className="rp-card">
        <div className="rp-eyebrow">CẬP NHẬT MỚI NHẤT</div>
        <h1>{r.project}</h1>
        <div className="rp-when">
          {fullTime(t)}
          {cats.map((c) => (
            <span key={c} className="rp-chip">
              {c}
            </span>
          ))}
          {r.gps && <span className="rp-chip ok">✓ GPS &amp; giờ chụp</span>}
        </div>
        <Mosaic photos={latest.photos} onOpen={(i) => setView({ set: latest, start: i })} />
        {text && <p className="rp-quote">“{text}”</p>}
        <div className="rp-progress">
          <div className="rp-prow">
            <span>Tiến độ công trình</span>
            <span className="v">{done ? 'Đã hoàn thành' : `${r.progress || 0}%`}</span>
          </div>
          <div className="rp-bar">
            <div style={{ width: `${done ? 100 : Math.min(100, r.progress || 0)}%` }} />
          </div>
          <small>
            Ngày {r.days} · {r.logs} nhật ký · {r.total} ảnh{r.videos ? ` · ${r.videos} video hôm nay` : ''}
          </small>
        </div>
      </section>

      {r.earlier?.length > 0 && (
        <section className="rp-card rp-earlier">
          <h2>Nhật ký trước đây</h2>
          {r.earlier.map((e) => {
            const et = new Date(e.taken_at)
            const en = splitNote(e.note)
            const set = { photos: e.photos, takenAt: e.taken_at, gps: e.gps, address: e.address || r.address }
            return (
              <div className="rp-log" key={e.taken_at + e.photos[0]}>
                <button type="button" className="rp-loghead" onClick={() => setView({ set, start: 0 })}>
                  <b className="num">{dayMonth(et)}</b>
                  <span>{en.text || en.cats.join(', ') || 'Nhật ký thi công'}</span>
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true"><path d="M9 6l6 6-6 6" /></svg>
                </button>
                <Mosaic photos={e.photos} max={3} small onOpen={(i) => setView({ set, start: i })} />
              </div>
            )
          })}
        </section>
      )}

      <div className="rp-foot">
        Ảnh được chụp trực tiếp trên app Vua Thầu,
        <br />
        có xác minh thời gian &amp; vị trí GPS.
      </div>

      {phone && (
        <div className="rp-actions">
          <a className="rp-call" href={`tel:${phone}`}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true">
              <path d="M5 4h4l2 5-3 2a11 11 0 0 0 5 5l2-3 5 2v4a2 2 0 0 1-2 2A17 17 0 0 1 3 6a2 2 0 0 1 2-2" />
            </svg>
            Gọi nhà thầu
          </a>
          <a className="rp-zalo" href={`https://zalo.me/${phone}`} target="_blank" rel="noreferrer">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true">
              <path d="M21 12a8 8 0 0 1-12 7l-5 1 1-4a8 8 0 1 1 16-4z" />
            </svg>
            Nhắn Zalo
          </a>
        </div>
      )}

      {view && <Viewer set={view.set} start={view.start} onClose={() => setView(null)} title={r.project} company={r.company} />}
    </main>
  )
}
