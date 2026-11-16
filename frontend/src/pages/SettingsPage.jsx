// Cấu hình (/settings, chỉ admin): giá hồ sơ, số ảnh tối thiểu, số hồ sơ để được thanh toán.
// Lưu xong app CTV đọc giá mới qua GET /config ở lần mở tới.
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import { reloadCfg } from '@/hooks/useCfg'
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
    ['pay_min', 'Số hồ sơ đã duyệt để được thanh toán', '1 đến 100'],
    ['dup_days', 'Chặn chụp trùng: số ngày 1 công trình chỉ nhận 1 hồ sơ', '1 đến 365 ngày'],
    ['dup_radius_m', 'Chặn chụp trùng: bán kính coi là cùng công trình', '10 đến 1000 mét. Trong bán kính này AI so ảnh để xác nhận'],
  ]
  return (
    <section className="stack">
      <PageHead title="Cấu hình" sub="Giá mới áp dụng cho hồ sơ gửi duyệt từ lúc lưu; hồ sơ đã gửi giữ giá cũ. App CTV tự cập nhật." />
      <InboxBox />
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
