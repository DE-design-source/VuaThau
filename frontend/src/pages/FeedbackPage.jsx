// Đánh giá app (/danh-gia, chỉ admin; góp ý 25/11/2026): CTV / nhà thầu chụp đủ 30 ảnh thì app hỏi 1-5 sao + góp ý.
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import adminApi from '@/api/adminApi'
import { PageHead, PageState } from '@/components/ui'
import { vnTime } from '@/utils/format'

const ROLE = { ctv: 'CTV', nhathau: 'Nhà thầu' }

export default function FeedbackPage() {
  const { data: d, err, reload } = usePage(() => adminApi.feedback())
  const [only, setOnly] = useState('') // '' | 'comment' | '1'..'5'
  if (!d) return <PageState err={err} retry={reload} />
  const rows = (d.items || []).filter((f) => (only === 'comment' ? f.comment : only ? String(f.stars) === only : true))

  return (
    <section className="stack">
      <PageHead
        title="Đánh giá app"
        sub="Cộng tác viên và nhà thầu tải lên đủ 30 ảnh thì app hỏi đánh giá 1-5 sao và góp ý. Bấm Để sau thì hỏi lại 1 lần khi đủ 60 ảnh."
      />
      <div className="card">
        {d.count ? (
          <p>
            <strong style={{ fontSize: '1.6em' }}>{d.avg.toFixed(1).replace('.', ',')}</strong> <span style={{ color: '#F5B301' }}>★</span>{' '}
            <span className="muted">trung bình từ {d.count} lượt đánh giá</span>
          </p>
        ) : (
          <p className="muted">Chưa có lượt đánh giá nào.</p>
        )}
        <div className="row-wrap">
          {[5, 4, 3, 2, 1].map((s) => (
            <button key={s} type="button" className={`btn ${only === String(s) ? 'btn-primary' : 'btn-secondary'}`} onClick={() => setOnly(only === String(s) ? '' : String(s))}>
              {s} ★ · {d.stars?.[s] || 0}
            </button>
          ))}
          <button type="button" className={`btn ${only === 'comment' ? 'btn-primary' : 'btn-secondary'}`} onClick={() => setOnly(only === 'comment' ? '' : 'comment')}>
            Có góp ý · {(d.items || []).filter((f) => f.comment).length}
          </button>
        </div>
      </div>
      {rows.length > 0 && (
        <div className="card">
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Thời gian</th>
                  <th>Người gửi</th>
                  <th>Đánh giá</th>
                  <th>Góp ý</th>
                  <th>Ảnh đã chụp</th>
                  <th>Phiên bản</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((f) => (
                  <tr key={f.id}>
                    <td className="nowrap">{vnTime(f.created_at)}</td>
                    <td>
                      <strong>{f.name}</strong>
                      <div className="muted">
                        {ROLE[f.role] || f.role} · {f.phone}
                      </div>
                    </td>
                    <td className="nowrap" style={{ color: '#F5B301' }} title={`${f.stars} sao`}>
                      {'★'.repeat(f.stars || 0)}
                      <span className="muted">{'★'.repeat(5 - (f.stars || 0))}</span>
                    </td>
                    <td style={{ whiteSpace: 'pre-wrap' }}>{f.comment || <span className="muted">(không ghi)</span>}</td>
                    <td>{f.photos}</td>
                    <td className="muted nowrap">
                      {f.version}
                      {f.platform ? ` · ${f.platform}` : ''}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  )
}
