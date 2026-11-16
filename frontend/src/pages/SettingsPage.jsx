// Cấu hình (/cau-hinh, chỉ admin): giá hồ sơ, số ảnh tối thiểu, số hồ sơ để được thanh toán.
// Lưu xong app CTV đọc giá mới qua /app.json ở lần mở tới.
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import { Button, PageHead, PageState } from '@/components/ui'

export default function SettingsPage() {
  const { data: d, err, reload } = usePage('/cau-hinh')
  if (!d) return <PageState err={err} retry={reload} />
  return <Form key={JSON.stringify(d.vals)} vals={d.vals} reload={reload} />
}

function Form({ vals, reload }) {
  const [act, busy] = useAct(reload)
  const [f, setF] = useState(() => Object.fromEntries(Object.entries(vals).map(([k, v]) => [k, String(v)])))
  const n = Number(f.min_photos) || vals.min_photos
  const fields = [
    ['min_photos', 'Số ảnh đạt tối thiểu của 1 hồ sơ', '1 đến 20'],
    ['price_min', `Tiền 1 hồ sơ đủ ${n} ảnh đạt`, 'đồng'],
    ['price_more', `Tiền 1 hồ sơ từ ${n + 1} ảnh đạt`, 'đồng'],
    ['pay_min', 'Số hồ sơ đã duyệt để được thanh toán', '1 đến 100'],
  ]
  return (
    <section className="stack">
      <PageHead title="Cấu hình" sub="Giá mới áp dụng cho hồ sơ gửi duyệt từ lúc lưu; hồ sơ đã gửi giữ giá cũ. App CTV tự cập nhật." />
      <form
        className="card stack narrow"
        onSubmit={(e) => {
          e.preventDefault()
          act('save', '/cau-hinh', f)
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
