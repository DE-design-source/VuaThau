// Nhà thầu dùng app (/nha-thau-app, admin): thêm tài khoản nhà thầu (đăng nhập app bằng SĐT), xem số dự án, nhật ký.
// Nhà thầu tự đăng dự án + chụp nhật ký thi công trên app (đợt 1, 23/11/2026).
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import ntApi from '@/api/ntApi'
import { Button, PageHead, PageState, Pill } from '@/components/ui'
import { vnDate, vnTime } from '@/utils/format'

export default function NtPage() {
  const { data: d, err, reload } = usePage(() => ntApi.list())
  const [act, busy] = useAct(reload)
  const [f, setF] = useState({ phone: '', name: '', company: '' })
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value })
  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []
  return (
    <section className="stack">
      <PageHead title="Nhà thầu dùng app" sub={`${rows.length} nhà thầu · tự đăng dự án và chụp nhật ký thi công trên app Vua Thầu Map`} />
      <form
        className="card stack narrow"
        onSubmit={(e) => {
          e.preventDefault()
          act('add', () => ntApi.add(f)).then((j) => j && setF({ phone: '', name: '', company: '' }))
        }}
      >
        <h2>Thêm nhà thầu</h2>
        <p className="muted">Nhà thầu mở app Vua Thầu Map, nhập số điện thoại này là vào giao diện nhà thầu (không cần mật khẩu).</p>
        <div className="grid-2 tight">
          <label className="field">
            <span>Số điện thoại</span>
            <input className="input" inputMode="tel" value={f.phone} onChange={set('phone')} placeholder="0908 111 222" />
          </label>
          <label className="field">
            <span>Người liên hệ</span>
            <input className="input" value={f.name} onChange={set('name')} placeholder="Anh Hùng" />
          </label>
        </div>
        <label className="field">
          <span>Tên công ty</span>
          <input className="input" value={f.company} onChange={set('company')} placeholder="Công ty TNHH Xây dựng Hùng Phát" />
        </label>
        <Button type="submit" loading={busy === 'add'} disabled={f.phone.replace(/\D/g, '').length < 10 || !f.name.trim()}>
          Thêm nhà thầu
        </Button>
      </form>
      <div className="card">
        {rows.length ? (
          <table className="table">
            <thead>
              <tr>
                <th>Nhà thầu</th>
                <th>Số điện thoại</th>
                <th className="num">Dự án</th>
                <th className="num">Nhật ký</th>
                <th>Cập nhật gần nhất</th>
                <th>Tạo lúc</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td>
                    <strong>{r.company || r.name}</strong> {r.locked === 1 && <Pill tone="bad">Đã khoá</Pill>}
                    {r.company && <small className="muted d-block">{r.name}</small>}
                  </td>
                  <td className="nowrap">{r.phone}</td>
                  <td className="num">{r.projects}</td>
                  <td className="num">{r.logs}</td>
                  <td className="nowrap">{r.last_log ? vnTime(r.last_log) : <span className="muted">Chưa có</span>}</td>
                  <td className="muted nowrap">{vnDate(r.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p className="muted">Chưa có nhà thầu nào. Thêm ở trên.</p>
        )}
      </div>
    </section>
  )
}
