// Cấu hình (/settings, chỉ admin): quy tắc duyệt ảnh cho AI, giá hồ sơ, số ảnh tối thiểu, số hồ sơ để được thanh toán.
// Lưu xong app CTV đọc giá mới qua GET /config ở lần mở tới.
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useCfg, { reloadCfg } from '@/hooks/useCfg'
import configApi from '@/api/configApi'
import mapApi from '@/api/mapApi'
import { Button, PageHead, PageState, Pill } from '@/components/ui'
import { parseMoney, vnTime } from '@/utils/format'

/** Hồ sơ CTV gửi qua fanpage Messenger (dezon-inbox): tự lấy mỗi 5 phút, admin bấm lấy ngay. */
function InboxBox() {
  const { data: s, reload } = usePage(() => mapApi.inbox())
  const [act, busy] = useAct(reload)
  if (!s) return null
  return (
    <div className="card stack narrow">
      <div className="row-between">
        <h2>Hồ sơ từ Messenger</h2>
        {s.enabled ? <Pill tone="ok">Đang tự lấy</Pill> : <Pill>Chưa kết nối</Pill>}
      </div>
      {s.enabled ? (
        <>
          <p className="muted">
            CTV gửi ảnh công trình cho fanpage, chatbot ghi nhận, hệ thống tự lấy về thành hồ sơ chờ duyệt (mỗi 5 phút). Người gửi chưa có
            tài khoản thì được tạo CTV mới; công trình đã có người chụp trong hạn thì tự từ chối.
          </p>
          <p>
            Đã lấy <strong>{s.total}</strong> hồ sơ.{' '}
            {s.at && (
              <span className="muted">
                Lần gần nhất {vnTime(s.at)}: {s.added} hồ sơ mới{s.rejected ? `, ${s.rejected} trùng công trình` : ''}.
              </span>
            )}
          </p>
          {s.error && <div className="note tone-bad">Lỗi lần gần nhất: {s.error}</div>}
          <Button loading={busy === 'sync' || s.running} onClick={() => act('sync', () => mapApi.syncInbox())}>
            Lấy hồ sơ mới ngay
          </Button>
        </>
      ) : (
        <p className="muted">Máy chủ chưa cấu hình kết nối dezon-inbox (INBOX_DATABASE_URL).</p>
      )}
    </div>
  )
}

export default function SettingsPage() {
  const { data: d, err, reload } = usePage(() => configApi.getSettings())
  if (!d) return <PageState err={err} retry={reload} />
  return <Form key={JSON.stringify(d)} vals={d} reload={reload} />
}

function Form({ vals, reload }) {
  const [act, busy] = useAct(reload)
  const [f, setF] = useState(() => Object.fromEntries(Object.entries(vals).map(([k, v]) => [k, String(v ?? '')])))
  const n = Number(f.min_photos) || vals.min_photos
  const fields = [
    ['min_photos', 'Số ảnh đạt tối thiểu của 1 hồ sơ', '1 đến 20'],
    ['price_min', `Tiền 1 hồ sơ đủ ${n} ảnh đạt`, 'đồng'],
    ['price_more', `Tiền 1 hồ sơ từ ${n + 1} ảnh đạt`, 'đồng'],
    ['video_bonus', 'Thưởng hồ sơ có video công trình', 'đồng, cộng thêm khi hồ sơ có ít nhất 1 video và đủ ảnh để được trả tiền'],
    ['price_out', 'Tiền tối đa hồ sơ chỉ chụp bên ngoài', 'đồng. Admin chọn "Chỉ chụp ngoài" / "Trong + ngoài" khi duyệt; hồ sơ Messenger mặc định chỉ ngoài'],
    ['pay_min', 'Số hồ sơ đã duyệt để được thanh toán', '1 đến 100'],
    ['dup_days', 'Chặn chụp trùng: số ngày 1 công trình chỉ nhận 1 hồ sơ', '1 đến 365 ngày'],
    ['dup_radius_m', 'Chặn chụp trùng: bán kính coi là cùng công trình', '10 đến 1000 mét. Trong bán kính này AI so ảnh để xác nhận'],
  ]
  return (
    <section className="stack">
      <PageHead title="Cấu hình" sub="Giá mới áp dụng cho hồ sơ gửi duyệt từ lúc lưu; hồ sơ đã gửi giữ giá cũ. App CTV tự cập nhật." />
      <AiRulesBox />
      <InboxBox />
      <KindsBox />
      <form
        className="card stack narrow"
        onSubmit={(e) => {
          e.preventDefault()
          const body = Object.fromEntries(fields.map(([k]) => [k, parseMoney(f[k])]))
          act('save', () => configApi.saveSettings(body)).then((j) => j && reloadCfg())
        }}
      >
        {fields.map(([k, label, hint]) => (
          <label key={k} className="field">
            <span>{label}</span>
            <input className="input" inputMode="numeric" value={f[k] ?? ''} onChange={(e) => setF({ ...f, [k]: e.target.value })} />
            <small className="muted">{hint}</small>
          </label>
        ))}
        <Button type="submit" loading={busy === 'save'}>
          Lưu cấu hình
        </Button>
      </form>
    </section>
  )
}

/** Danh sách chức năng CTV (cột Chức năng, ô lọc ở trang Cộng tác viên): mỗi dòng 1 chức năng. */
/** Quy tắc duyệt ảnh cho AI (góp ý 21/11/2026): admin viết thêm, AI nhận kèm lời dặn mặc định từ lượt đọc ảnh tiếp theo.
 *  Lời dặn mặc định (khung kết quả, cách đọc giờ / địa chỉ) giữ nguyên, chỉ xem để tham khảo. */
function AiRulesBox() {
  const { data: d, err, reload } = usePage(() => configApi.getAiRules())
  const [act, busy] = useAct(reload)
  const [v, setV] = useState(null) // null: chưa sửa
  if (!d) return err ? <div className="card narrow text-bad">{err}</div> : null
  const text = v ?? d.rules ?? ''
  const max = d.max || 8000
  return (
    <form
      className="card stack"
      onSubmit={(e) => {
        e.preventDefault()
        act('ai', () => configApi.saveAiRules(text)).then((j) => j && setV(null))
      }}
    >
      <h2>Quy tắc duyệt ảnh cho AI</h2>
      <p className="muted">
        Viết thêm quy tắc để AI đọc và đánh giá ảnh (ảnh có phải công trình không, chụp trong hay ngoài, giai đoạn, nhà thầu...). AI làm theo quy tắc này
        từ lượt đọc ảnh tiếp theo; nếu mâu thuẫn với lời dặn mặc định thì ưu tiên quy tắc ở đây. Để trống: AI dùng lời dặn mặc định.
      </p>
      <textarea
        className="input mono-ish"
        rows={10}
        maxLength={max}
        value={text}
        placeholder={'Ví dụ:\n- Ảnh chụp bảng giấy phép xây dựng cũng tính là ảnh công trình.\n- Ảnh chụp từ trên cao (flycam) vẫn tính là chụp ngoài.'}
        onChange={(e) => setV(e.target.value)}
      />
      <div className="row-between">
        <small className="muted">
          {text.length}/{max} ký tự{d.at ? ` · sửa lần cuối ${vnTime(d.at)}` : ''}
        </small>
        <div className="row-wrap">
          {v !== null && (
            <Button variant="ghost" type="button" onClick={() => setV(null)}>
              Huỷ sửa
            </Button>
          )}
          <Button type="submit" loading={busy === 'ai'} disabled={v === null || v.trim() === (d.rules || '')}>
            Lưu quy tắc
          </Button>
        </div>
      </div>
      <details>
        <summary className="muted">Xem lời dặn mặc định AI đang dùng</summary>
        <pre className="ai-prompt">{d.default_prompt}</pre>
      </details>
    </form>
  )
}

function KindsBox() {
  const cfg = useCfg()
  const [act, busy] = useAct(reloadCfg)
  const [v, setV] = useState(null) // null: chưa sửa, hiện theo cấu hình
  if (!cfg) return null
  const text = v ?? (cfg.ctv_kinds || ['CTV']).join('\n')
  const list = text.split('\n').map((x) => x.trim()).filter(Boolean)
  return (
    <form
      className="card stack narrow"
      onSubmit={(e) => {
        e.preventDefault()
        act('kinds', () => configApi.saveKinds(list)).then((j) => j && setV(null))
      }}
    >
      <h2>Chức năng cộng tác viên</h2>
      <p className="muted">Mỗi dòng 1 chức năng, dùng cho cột Chức năng và ô lọc ở trang Cộng tác viên. CTV mới mặc định là "CTV", nên giữ dòng này.</p>
      <textarea className="input" rows={Math.max(3, list.length + 1)} value={text} onChange={(e) => setV(e.target.value)} />
      <Button type="submit" loading={busy === 'kinds'} disabled={v === null || !list.length}>
        Lưu danh sách chức năng
      </Button>
    </form>
  )
}
