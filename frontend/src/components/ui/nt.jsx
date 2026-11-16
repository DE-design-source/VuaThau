// Mảnh giao diện dùng chung cho web quản lý Nghiệm thu.
import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link } from 'react-router-dom'
import ntClient from '@/api/ntClient'
import { clearToast, currentToast, subscribeToast } from '@/services/toast'
import useCfg from '@/hooks/useCfg'
import Loader from './Loader'
import Button from './Button'

export function Toasts() {
  const msgs = useSyncExternalStore(subscribeToast, currentToast)
  if (!msgs.length) return null
  return (
    <div className="toast" role="alert" onClick={clearToast}>
      {msgs.map((m, i) => (
        <p key={i}>{m}</p>
      ))}
    </div>
  )
}

/** Trang chưa tải xong / lỗi. */
export function PageState({ err, retry }) {
  return (
    <div className="page-state">
      {err ? (
        <>
          <p className="muted">{err}</p>
          {retry && (
            <Button variant="secondary" onClick={retry}>
              Thử lại
            </Button>
          )}
        </>
      ) : (
        <Loader size="lg" text="Đang tải…" />
      )}
    </div>
  )
}

export function PageHead({ title, sub, children }) {
  return (
    <div className="page-head">
      <div>
        <h1>{title}</h1>
        {sub && <p className="muted">{sub}</p>}
      </div>
      {children && <div className="page-actions">{children}</div>}
    </div>
  )
}

export function Kpi({ label, value, sub, tone }) {
  return (
    <div className={`kpi ${tone ? `kpi-${tone}` : ''}`}>
      <span className="muted">{label}</span>
      <strong>{value}</strong>
      {sub && <small className="muted">{sub}</small>}
    </div>
  )
}

/** Nhãn trạng thái hồ sơ (nhãn nội bộ, cùng tông màu với app). */
export function StatusBadge({ status }) {
  const cfg = useCfg()
  const [label, , tone] = cfg?.STATUS?.[status] ?? [status, '', 'draft']
  return <span className={`badge tone-${tone}`}>{label}</span>
}

export function Pill({ tone = 'draft', children }) {
  return <span className={`badge tone-${tone}`}>{children}</span>
}

export function Empty({ title, text }) {
  return (
    <div className="empty">
      <strong>{title}</strong>
      {text && <p className="muted">{text}</p>}
    </div>
  )
}

export function Pager({ trang, pages, onGo }) {
  if (pages <= 1) return null
  return (
    <nav className="pager" aria-label="Phân trang">
      <Button variant="secondary" disabled={trang <= 1} onClick={() => onGo(trang - 1)}>
        ‹ Trước
      </Button>
      <span className="muted">
        Trang {trang} / {pages}
      </span>
      <Button variant="secondary" disabled={trang >= pages} onClick={() => onGo(trang + 1)}>
        Sau ›
      </Button>
    </nav>
  )
}

/** Ảnh cần đăng nhập (ảnh hồ sơ, biên lai): tải bằng token rồi hiện. */
export function AuthImage({ path, alt = '', className = '', onClick }) {
  const [src, setSrc] = useState(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let url = null
    let dead = false
    ntClient.blob(path).then(
      (b) => {
        url = URL.createObjectURL(b)
        if (dead) URL.revokeObjectURL(url)
        else setSrc(url)
      },
      () => !dead && setFailed(true),
    )
    return () => {
      dead = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [path])
  if (failed) return <div className={`img-ph ${className}`}>Không tải được ảnh</div>
  if (!src) return <div className={`img-ph ${className}`} />
  return <img src={src} alt={alt} className={className} onClick={onClick} loading="lazy" />
}

/** Ảnh bìa nhỏ của hồ sơ trong danh sách. */
export function Cover({ st }) {
  return st?.cover ? <AuthImage path={`/anh/${st.cover}`} className="thumb" /> : <div className="thumb img-ph" />
}

/** Lưới ảnh hồ sơ + xem lớn (vuốt bằng phím ← →). */
export function PhotoGrid({ ps }) {
  const [at, setAt] = useState(null)
  useEffect(() => {
    if (at === null) return
    const key = (e) => {
      if (e.key === 'Escape') setAt(null)
      if (e.key === 'ArrowRight') setAt((k) => Math.min(ps.length - 1, k + 1))
      if (e.key === 'ArrowLeft') setAt((k) => Math.max(0, k - 1))
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [at, ps.length])
  const p = at !== null ? ps[at] : null
  const lat = p?.lat
  return (
    <>
      <div className="photo-grid">
        {ps.map((x, k) => (
          <button key={x.id} type="button" className={`photo ${x.problems?.length ? 'photo-bad' : ''}`} onClick={() => setAt(k)}>
            <AuthImage path={`/anh/${x.id}`} alt={`Ảnh ${k + 1}`} />
            <span className={`photo-mark ${x.problems?.length ? 'bad' : 'ok'}`}>{x.problems?.length ? '✕' : '✓'}</span>
            <span className="photo-n">{k + 1}</span>
          </button>
        ))}
      </div>
      {p && (
        <div className="lightbox" role="dialog" aria-modal="true" onClick={() => setAt(null)}>
          <div className="lightbox-bar" onClick={(e) => e.stopPropagation()}>
            <strong>
              Ảnh {at + 1} / {ps.length}
            </strong>
            <span className="spacer" />
            <Button variant="ghost" disabled={at === 0} onClick={() => setAt(at - 1)}>
              ‹
            </Button>
            <Button variant="ghost" disabled={at === ps.length - 1} onClick={() => setAt(at + 1)}>
              ›
            </Button>
            <a className="btn btn-ghost" href="#goc" onClick={(e) => { e.preventDefault(); ntClient.download(`/anh/${p.id}?goc=1`, `anh-${p.id}.jpg`) }}>
              Tải ảnh gốc
            </a>
            <Button variant="ghost" onClick={() => setAt(null)} aria-label="Đóng">
              ✕
            </Button>
          </div>
          <div className="lightbox-img" onClick={(e) => e.stopPropagation()}>
            <AuthImage key={p.id} path={`/anh/${p.id}`} alt={`Ảnh ${at + 1}`} />
          </div>
          <div className="lightbox-info" onClick={(e) => e.stopPropagation()}>
            <span>🕒 {p.exif_time || p.ai_time || 'Không đọc được giờ chụp'}</span>
            <span>
              📍 {p.address || (lat != null ? `${lat.toFixed(5)}, ${p.lng.toFixed(5)}` : 'Không có vị trí')}
              {lat != null && (
                <>
                  {' '}
                  <a href={`https://maps.google.com/?q=${lat},${p.lng}`} target="_blank" rel="noreferrer">
                    Bản đồ
                  </a>
                </>
              )}
            </span>
            {p.problems?.length ? (
              p.problems.map((x) => (
                <span key={x} className="text-bad">
                  ✕ {x}
                </span>
              ))
            ) : (
              <span className="text-ok">✓ Ảnh đạt</span>
            )}
          </div>
        </div>
      )}
    </>
  )
}

/** Bảng hồ sơ dùng ở nhiều trang. */
export function HouseTable({ rows, stats, showCtv = true, selectable, selected, onToggle }) {
  if (!rows.length) return <Empty title="Không có hồ sơ nào" />
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            {selectable && <th />}
            <th>Hồ sơ</th>
            {showCtv && <th>CTV</th>}
            <th>Ảnh đạt</th>
            <th>Trạng thái</th>
            <th className="num">Tiền</th>
            <th>Ngày tạo</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const st = stats?.[r.id] ?? stats?.[String(r.id)]
            return (
              <tr key={r.id}>
                {selectable && (
                  <td>
                    <input type="checkbox" checked={selected.has(r.id)} onChange={() => onToggle(r.id)} aria-label={`Chọn hồ sơ #${r.id}`} />
                  </td>
                )}
                <td>
                  <Link to={`/nha/${r.id}`} className="row-link">
                    <Cover st={st} />
                    <span>
                      <strong>#{r.id}</strong> {r.address || <span className="muted">(chưa có địa chỉ)</span>}
                      {r.dup && <small className="text-warn d-block">⚠ {r.dup}</small>}
                    </span>
                  </Link>
                </td>
                {showCtv && (
                  <td>
                    <Link to={`/ctv/${r.ctv_id}`}>{r.ctv_name}</Link>
                  </td>
                )}
                <td>{st ? `${st.valid}/${st.n}` : '—'}</td>
                <td>
                  <StatusBadge status={r.status} />
                </td>
                <td className="num">{r.amount ? `${r.amount.toLocaleString('vi-VN')}đ` : '—'}</td>
                <td className="muted">{r.created_at?.slice(0, 10).split('-').reverse().join('/')}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
