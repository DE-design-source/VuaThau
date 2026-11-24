// Hồ sơ năng lực công khai của nhà thầu (/nt/:code): nhà thầu bấm "Chia sẻ" ở trang Dự án trong app, gửi link cho khách.
// Công ty, khu vực, chuyên môn; các dự án (ảnh bìa, tình trạng, tiến độ) -> bấm xem báo cáo nhật ký mới nhất (/bc/...).
// Không cần đăng nhập; cùng kiểu trang báo cáo (publicReport.css).
import { useEffect } from 'react'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import publicApi from '@/api/publicApi'
import './publicReport.css'

function initials(name = '') {
  let n = ` ${name.toLowerCase()} `
  for (const w of ['công ty', 'cty', 'tnhh', 'cổ phần', 'cp', 'xây dựng', 'xd', 'thương mại', 'dịch vụ', 'kiến trúc', 'nội thất', 'thiết kế', 'mtv', 'và', '&'])
    n = n.split(` ${w} `).join(' ')
  const words = (n.trim() || name).split(/\s+/).filter(Boolean)
  if (!words.length) return 'NT'
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[words.length - 1][0]).toUpperCase()
}

export default function PortfolioPage() {
  const { code } = useParams()
  const { data: r, err } = usePage(() => publicApi.portfolio(code), [code])

  useEffect(() => {
    if (r) document.title = `${r.company || 'Nhà thầu'} · Hồ sơ năng lực`
  }, [r])

  if (!r) {
    return (
      <main className="rp">
        <div className="rp-empty">{err ? err : 'Đang mở hồ sơ…'}</div>
      </main>
    )
  }
  const phone = (r.phone || '').replace(/\D/g, '')
  return (
    <main className="rp">
      <header className="rp-top">
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
          <small>Hồ sơ năng lực{r.verified ? ' · Vua Thầu đã xác minh' : ''}</small>
        </div>
      </header>

      <section className="rp-card">
        <div className="rp-pfstats">
          <div>
            <b>{r.projects.length}</b>
            <span>Dự án</span>
          </div>
          <div>
            <b>{r.logs}</b>
            <span>Nhật ký</span>
          </div>
          <div>
            <b>{r.years || '—'}</b>
            <span>Năm KN</span>
          </div>
          <div>
            <b>{r.views}</b>
            <span>Lượt xem</span>
          </div>
        </div>
        {r.specialties?.length > 0 && (
          <div className="rp-when" style={{ marginTop: 10 }}>
            {r.specialties.map((s) => (
              <span key={s} className="rp-chip">
                {s}
              </span>
            ))}
          </div>
        )}
      </section>

      <section className="rp-card">
        <h2>Dự án</h2>
        {r.projects.length === 0 && <p className="rp-quote">Nhà thầu chưa đăng dự án nào.</p>}
        <div className="rp-pfgrid">
          {r.projects.map((p) => {
            const done = p.status === 'hoan_thanh'
            const body = (
              <>
                <div className="rp-pfcover">
                  {p.cover_url ? <img src={`${p.cover_url}${p.cover_url.includes('?') ? '&' : '?'}size=small`} alt="" loading="lazy" /> : <i />}
                  <span className={`rp-pfstate${done ? ' done' : ''}`}>{done ? 'Hoàn thành' : 'Đang thi công'}</span>
                  <span className="rp-pfcount">{p.photos} ảnh</span>
                </div>
                <b>{p.name}</b>
                <small>
                  {[p.address.split(',').slice(-2).join(',').trim(), done && p.finished_on ? `xong ${new Date(p.finished_on).getFullYear()}` : `${p.days} ngày`]
                    .filter(Boolean)
                    .join(' · ')}
                </small>
              </>
            )
            return p.report ? (
              <Link key={p.name + p.report} to={p.report} className="rp-pfitem">
                {body}
              </Link>
            ) : (
              <div key={p.name} className="rp-pfitem">
                {body}
              </div>
            )
          })}
        </div>
      </section>

      <div className="rp-foot">
        Dự án và ảnh do nhà thầu cập nhật trên app Vua Thầu,
        <br />
        ảnh có xác minh thời gian &amp; vị trí GPS.
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
    </main>
  )
}
