// Chi tiết 1 nhà thầu (/contractors/:key, góp ý 23/11/2026, theo thiết kế "Chi tiết nhà thầu"):
//   đầu trang: logo chữ tắt, tên, trạng thái xác minh, liên hệ; số công trình / đang thi công / hoàn thành;
//   hồ sơ năng lực (vai trò, kinh nghiệm, quy mô, giá trị, khu vực, lĩnh vực) từ form đăng ký;
//   danh sách dự án CTV đã chụp: tab Tất cả / Đang thi công / Hoàn thành, lọc theo bước thi công.
import { useState } from 'react'
import useAuth from '@/hooks/useAuth'
import { Link, useParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import contractorApi from '@/api/contractorApi'
import { Icon, PageState } from '@/components/ui'
import SiteCard from '@/components/explore/SiteCard'
import { cardProps } from '@/utils/siteTip'
import { gmapUrl, initials, vnDate } from '@/utils/format'

const DONE = 'Bàn giao'
const STEPS = ['Chuẩn bị mặt bằng', 'Ép cọc, đào móng', 'Thi công móng', 'Xây phần thô', 'Xây tường, đổ mái', 'Điện nước âm tường', 'Hoàn thiện', 'Bàn giao']
const done = (p) => (p.stage || '').trim() === DONE

function Fact({ label, children }) {
  return (
    <div className="nt-fact">
      <small>{label}</small>
      <b>{children || <span className="muted">—</span>}</b>
    </div>
  )
}

export default function ContractorPage() {
  const { key } = useParams()
  const { data: d, err, reload } = usePage(() => contractorApi.get(key), [key])
  const [tab, setTab] = useState('all')
  const [step, setStep] = useState('')
  const { user } = useAuth()
  const [verifying, setVerifying] = useState(false)
  const [verifyMsg, setVerifyMsg] = useState('')
  if (!d) return <PageState err={err} retry={reload} />
  const pf = d.profile || {}
  const ps = d.projects || []
  const nDone = ps.filter(done).length
  const inTab = ps.filter((p) => (tab === 'all' ? true : tab === 'done' ? done(p) : !done(p)))
  const rows = step ? inTab.filter((p) => (p.stage || '').trim() === step) : inTab
  const tel = (d.phone || '').replace(/[^\d+]/g, '')
  const first = ps.find((p) => p.lat != null)
  const tabs = [
    ['all', 'Tất cả', ps.length],
    ['run', 'Đang thi công', ps.length - nDone],
    ['done', 'Hoàn thành', nDone],
  ]

  return (
    <section className="stack">
      <Link className="nt-back" to="/contractors">
        ‹ Danh sách nhà thầu
      </Link>
      <div className="card nt-hero">
        <div className="nt-band" />
        <div className="nt-hero-main">
          <span className={`nt-logo ${d.status === 'Đã xác minh' ? 'on' : ''}`}>{initials(d.name)}</span>
          <div className="nt-hero-info">
            <h1>{d.name || d.phone || 'Nhà thầu'}</h1>
            <p className="nt-meta">
              <span className={`badge tone-${d.status === 'Đã xác minh' ? 'ok' : d.registered ? 'draft' : 'wait'}`}>{d.status}</span>
              {d.kind && <span>{d.kind}</span>}
              <span className="muted">{d.registered ? `Tham gia ${vnDate(d.joined).slice(3)}` : `CTV chụp từ ${vnDate(d.joined)}`}</span>
            </p>
            {/* nhà thầu tự đăng ký trong app: admin xác minh ở đây (đăng ký qua form Lark thì tích trên Lark) */}
            {d.from_app && d.status !== 'Đã xác minh' && user?.role === 'admin' && (
              <p className="nt-verify">
                <span className="muted">Tự đăng ký trong app, chờ xác minh.</span>
                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={verifying}
                  onClick={async () => {
                    setVerifying(true)
                    try {
                      const r = await contractorApi.verify(d.key)
                      setVerifyMsg(r?.data?.message || 'Đã xác minh.')
                      reload()
                    } catch (e) {
                      setVerifyMsg(e.message)
                    } finally {
                      setVerifying(false)
                    }
                  }}
                >
                  {verifying ? 'Đang xác minh…' : 'Xác minh & cấp tài khoản'}
                </button>
              </p>
            )}
            {verifyMsg && <p className="muted">{verifyMsg}</p>}
            <p className="nt-contact">
              {pf.contact && (
                <span>
                  <Icon name="users" size={14} /> Đại diện: <b>{pf.contact}</b>
                </span>
              )}
              {d.phone && (
                <span>
                  📞 <a href={`tel:${tel}`}>{d.phone}</a> <em className="nt-zalo">Zalo</em>
                </span>
              )}
              {pf.email && <span>✉ {pf.email}</span>}
            </p>
          </div>
          <div className="nt-hero-side">
            <div className="nt-actions">
              {first && (
                <a className="btn btn-secondary" href={gmapUrl(first.lat, first.lng)} target="_blank" rel="noreferrer">
                  <Icon name="map" size={16} /> Xem bản đồ
                </a>
              )}
              {tel && (
                <a className="btn nt-add" href={`tel:${tel}`}>
                  Gọi nhà thầu
                </a>
              )}
            </div>
            <div className="nt-stats">
              <div>
                <b>{ps.length}</b>
                <small>Công trình</small>
              </div>
              <div>
                <b>{ps.length - nDone}</b>
                <small>Đang thi công</small>
              </div>
              <div>
                <b>{nDone}</b>
                <small>Hoàn thành</small>
              </div>
            </div>
          </div>
        </div>
        <div className="nt-facts">
          <Fact label="Vai trò">{d.role}</Fact>
          <Fact label="Kinh nghiệm">{d.exp}</Fact>
          <Fact label="Quy mô nhân sự">{pf.staff}</Fact>
          <Fact label="Giá trị công trình">{d.value_range}</Fact>
          <Fact label="Khu vực thi công">{d.areas}</Fact>
        </div>
        {(pf.fields || []).length > 0 && (
          <div className="nt-fields">
            <small>Lĩnh vực thi công</small>
            {pf.fields.map((x) => (
              <span key={x} className="nt-chip">
                {x}
              </span>
            ))}
          </div>
        )}
        {!d.registered && <p className="muted nt-note">Nhà thầu này chưa đăng ký qua form Vua Thầu: thông tin lấy từ bảng hiệu CTV chụp tại công trình.</p>}
        {pf.note && <p className="muted nt-note">Ghi chú: {pf.note}</p>}
      </div>

      <div className="nt-proj-head">
        <h2>Danh sách dự án</h2>
        <div className="nt-tabs">
          {tabs.map(([k, label, n]) => (
            <button type="button" key={k} className={tab === k ? 'on' : ''} onClick={() => setTab(k)}>
              {label} · {n}
            </button>
          ))}
        </div>
      </div>
      {ps.length > 0 && (
        <div className="nt-steps">
          <span className="muted">Lọc theo bước thi công</span>
          {STEPS.map((s) => {
            const n = inTab.filter((p) => (p.stage || '').trim() === s).length
            return n ? (
              <button type="button" key={s} className={`chip ${step === s ? 'chip-on' : ''}`} onClick={() => setStep(step === s ? '' : s)}>
                {s} <b>{n}</b>
              </button>
            ) : null
          })}
          {step && (
            <button type="button" className="cus-clear" onClick={() => setStep('')}>
              Bỏ lọc bước
            </button>
          )}
        </div>
      )}
      {rows.length ? (
        <div className="ex-grid">
          {rows.map((p) => (
            <SiteCard
              key={p.id}
              p={p}
              {...cardProps({ ...p, last_at: p.at }, { to: `/houses/${p.id}`, prov: p.province_name })}
              contractor={`Mã HS-${String(p.id).padStart(5, '0')} · ${p.photos} ảnh`}
              onLocate={() => p.lat != null && window.open(gmapUrl(p.lat, p.lng), '_blank', 'noopener')}
            />
          ))}
        </div>
      ) : (
        <p className="muted">{ps.length ? 'Không có dự án theo bộ lọc.' : 'Chưa có công trình nào CTV chụp của nhà thầu này.'}</p>
      )}
    </section>
  )
}
