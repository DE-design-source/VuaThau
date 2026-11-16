// Mảnh giao diện dùng chung cho web quản lý Nghiệm thu.
import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link } from 'react-router-dom'
import { getBlob } from '@/api/axiosClient'
import houseApi from '@/api/houseApi'
import { clearToast, currentToast, subscribeToast, toast } from '@/services/toast'
import { saveBlob } from '@/utils/download'
import { aiDiffText, gmapUrl, hsId, vnDate } from '@/utils/format'
import useCfg from '@/hooks/useCfg'
import Loader from './Loader'
import Button from './Button'
import Icon from './Icon'

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
      <span className="kpi-label">{label}</span>
      <strong className="mono">{value}</strong>
      {sub && <small className="muted">{sub}</small>}
    </div>
  )
}

/** Nhãn trạng thái hồ sơ (nhãn nội bộ, cùng tông màu với app). */
export function StatusBadge({ status }) {
  const cfg = useCfg()
  const st = cfg?.statuses?.[status]
  const label = st?.label || status
  const tone = st?.tone || 'draft'
  return (
    <span className={`badge tone-${tone}`}>
      <i className="dot" />
      {label}
    </span>
  )
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

export function Pager({ page, pages, onGo }) {
  if (pages <= 1) return null
  return (
    <nav className="pager" aria-label="Phân trang">
      <Button variant="secondary" disabled={page <= 1} onClick={() => onGo(page - 1)}>
        <Icon name="left" size={16} /> Trước
      </Button>
      <span className="muted mono">
        {page} / {pages}
      </span>
      <Button variant="secondary" disabled={page >= pages} onClick={() => onGo(page + 1)}>
        Sau <Icon name="right" size={16} />
      </Button>
    </nav>
  )
}

/** Ảnh cần đăng nhập (ảnh hồ sơ, biên lai): tải qua axiosClient (Bearer) dạng Blob rồi hiện. path: đường dẫn API, vd /photos/101. */
export function AuthImage({ path, alt = '', className = '', onClick }) {
  const [src, setSrc] = useState(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let url = null
    let dead = false
    getBlob(path).then(
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
export function Cover({ id }) {
  return id ? <AuthImage path={`/photos/${id}`} className="thumb" /> : <div className="thumb img-ph" />
}

/** Tải ảnh gốc về máy. */
function downloadOriginal(id) {
  houseApi.photo(id, { original: true }).then((b) => saveBlob(b, `anh-${id}.jpg`), (e) => toast(e.message))
}

/** Lưới ảnh hồ sơ + xem lớn (vuốt bằng phím ← →). */
export function PhotoGrid({ ps, checks = true }) {
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
            <AuthImage path={`/photos/${x.id}`} alt={`Ảnh ${k + 1}`} />
            {checks && <span className={`photo-mark ${x.problems?.length ? 'bad' : 'ok'}`}><Icon name={x.problems?.length ? 'x' : 'check'} size={13} /></span>}
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
              <Icon name="left" />
            </Button>
            <Button variant="ghost" disabled={at === ps.length - 1} onClick={() => setAt(at + 1)}>
              <Icon name="right" />
            </Button>
            <a className="btn btn-ghost" href="#goc" onClick={(e) => { e.preventDefault(); downloadOriginal(p.id) }}>
              <Icon name="download" size={16} /> Ảnh gốc
            </a>
            <Button variant="ghost" onClick={() => setAt(null)} aria-label="Đóng">
              <Icon name="x" />
            </Button>
          </div>
          <div className="lightbox-img" onClick={(e) => e.stopPropagation()}>
            <AuthImage key={p.id} path={`/photos/${p.id}`} alt={`Ảnh ${at + 1}`} />
          </div>
          <div className="lightbox-info" onClick={(e) => e.stopPropagation()}>
            <span className="inline"><Icon name="clock" size={15} /> <span className="mono">{p.exif_time || p.ai_time || 'Không đọc được giờ chụp'}</span>
              {p.exif_time && p.ai_time && p.ai_time !== p.exif_time && <span className="muted"> · giờ in trên ảnh (AI đọc): <span className="mono">{p.ai_time}</span></span>}
            </span>
            {p.is_site && (
              <span className="inline muted">
                <Icon name="check" size={14} /> AI thấy công trình đang xây
              </span>
            )}
            <span className="inline">
              <Icon name="pin" size={15} /> {p.address || (lat != null ? `${lat.toFixed(5)}, ${p.lng.toFixed(5)}` : 'Không có vị trí')}
              {lat != null && (
                <>
                  {' '}
                  <a href={gmapUrl(lat, p.lng)} target="_blank" rel="noreferrer">
                    Google Maps
                  </a>
                </>
              )}
            </span>
            {p.problems?.length ? (
              p.problems.map((x) => (
                <span key={x} className="text-bad">
                  <Icon name="x" size={14} /> {x}
                </span>
              ))
            ) : (
              checks && <span className="text-ok inline"><Icon name="check" size={14} /> Ảnh đạt</span>
            )}
          </div>
        </div>
      )}
    </>
  )
}

/** Bảng hồ sơ dùng ở nhiều trang. */
export function HouseTable({ rows, showCtv = true, selectable, selected, onToggle }) {
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
            return (
              <tr key={r.id}>
                {selectable && (
                  <td>
                    <input type="checkbox" checked={selected.has(r.id)} onChange={() => onToggle(r.id)} aria-label={`Chọn hồ sơ #${r.id}`} />
                  </td>
                )}
                <td>
                  <Link to={`/houses/${r.id}`} className="row-link">
                    <Cover id={r.cover_id} />
                    <span>
                      <span className="id">{hsId(r.id)}</span>
                      {r.vip && <span className="vip-tag" title="CTV chụp công trình này đầu tiên">VIP</span>}
                      {r.source === 'inbox' && <span className="src">Messenger</span>}
                      <span className="d-block row-title">{r.address || <span className="muted">(chưa có địa chỉ)</span>}</span>
                      {r.dup && (
                        <small className="text-warn d-block inline">
                          <Icon name="alert" size={13} /> {r.dup}
                        </small>
                      )}
                      {r.ai_diff && (
                        <small className="text-warn d-block inline">
                          <Icon name="alert" size={13} /> AI đọc trên ảnh khác {aiDiffText(r.ai_diff)}
                        </small>
                      )}
                    </span>
                  </Link>
                </td>
                {showCtv && (
                  <td>
                    <Link to={`/ctvs/${r.ctv_id}`} className="ctv-name">
                      {r.ctv_name}
                    </Link>
                  </td>
                )}
                <td className="mono">{r.photos_total != null ? `${r.photos_valid ?? 0}/${r.photos_total}` : '—'}</td>
                <td>
                  <StatusBadge status={r.status} />
                </td>
                <td className="num mono">{r.amount ? `${r.amount.toLocaleString('vi-VN')}đ` : '—'}</td>
                <td className="muted mono">{vnDate(r.created_at)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
