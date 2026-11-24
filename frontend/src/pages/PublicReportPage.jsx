// Báo cáo thi công gửi chủ nhà (/bc/:code, thiết kế 24/11/2026): nhà thầu gửi link sau khi tải nhật ký lên.
// Không cần đăng nhập, không menu quản lý; xem trên điện thoại là chính. Không SĐT / tài khoản nhà thầu.
import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import publicApi from '@/api/publicApi'
import { Mark } from '@/components/layout/Header'

const p2 = (n) => String(n).padStart(2, '0')
const WD = ['Chủ Nhật', 'Thứ Hai', 'Thứ Ba', 'Thứ Tư', 'Thứ Năm', 'Thứ Sáu', 'Thứ Bảy']

export default function PublicReportPage() {
  const { code } = useParams()
  const { data: r, err } = usePage(() => publicApi.report(code), [code])
  const [big, setBig] = useState(-1)

  useEffect(() => {
    if (r) document.title = `${r.project} · Nhật ký thi công`
  }, [r])

  useEffect(() => {
    if (big < 0) return undefined
    const onKey = (e) => {
      if (e.key === 'Escape') setBig(-1)
      if (e.key === 'ArrowRight') setBig((i) => Math.min(i + 1, (r?.photos?.length || 1) - 1))
      if (e.key === 'ArrowLeft') setBig((i) => Math.max(i - 1, 0))
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [big, r])

  if (!r) {
    return (
      <main className="bc">
        <div className="bc-empty">{err ? err : 'Đang mở báo cáo…'}</div>
      </main>
    )
  }
  const t = new Date(r.taken_at)
  const photos = r.photos || []
  return (
    <main className="bc">
      <header className="bc-hero">
        {photos[0] && <img src={`${photos[0]}?size=small`} alt="" />}
        <div className="bc-hero-text">
          <span className="bc-eyebrow">{(r.company || 'Nhà thầu').toUpperCase()} · NHẬT KÝ THI CÔNG</span>
          <h1>{r.project}</h1>
          {r.address && <p>{r.address}</p>}
        </div>
      </header>

      <section className="bc-body">
        <div className="bc-meta">
          <span>
            {WD[t.getDay()]}, {p2(t.getDate())}/{p2(t.getMonth() + 1)}/{t.getFullYear()} · {p2(t.getHours())}:{p2(t.getMinutes())}
          </span>
          {r.gps && <span className="bc-chip ok">✓ Xác minh GPS &amp; giờ chụp</span>}
          <span className="bc-chip">
            {photos.length} ảnh{r.videos ? ` · ${r.videos} video` : ''}
          </span>
          {r.status !== 'hoan_thanh' && r.progress > 0 && <span className="bc-chip">Tiến độ {r.progress}%</span>}
          {r.status === 'hoan_thanh' && <span className="bc-chip ok">Đã hoàn thành</span>}
        </div>

        {r.note && (
          <div className="bc-note">
            <b>Hôm nay đội đã làm</b>
            <p>{r.note}</p>
          </div>
        )}

        <div className="bc-grid">
          {photos.map((u, i) => (
            <button type="button" key={u} onClick={() => setBig(i)} aria-label={`Xem ảnh ${i + 1}`}>
              <img src={`${u}?size=small`} alt="" loading="lazy" />
            </button>
          ))}
        </div>

        <footer className="bc-foot">
          <Mark size={28} />
          <span>
            Ảnh chụp bằng camera Vua Thầu Map: in giờ chụp và vị trí GPS lên ảnh, không chỉnh sửa được.
            {r.company && (
              <>
                {' '}
                Nhà thầu: <b>{r.company}</b>
                {r.verified ? ' (đã xác minh)' : ''}.
              </>
            )}
          </span>
        </footer>
      </section>

      {big >= 0 && photos[big] && (
        <div className="bc-viewer" role="dialog" aria-label="Xem ảnh" onClick={() => setBig(-1)}>
          <img src={photos[big]} alt={`Ảnh ${big + 1}`} />
          <span className="bc-count">
            {big + 1}/{photos.length}
          </span>
        </div>
      )}
    </main>
  )
}
