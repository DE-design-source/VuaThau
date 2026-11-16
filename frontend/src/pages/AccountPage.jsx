// Tài khoản nhân viên (/me): thông tin, đổi mật khẩu.
import { useState } from 'react'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import useAuth from '@/hooks/useAuth'
import userApi from '@/api/userApi'
import { Button, PageHead, PageState } from '@/components/ui'
import { ROLE_LABEL } from '@/utils/format'

export default function AccountPage() {
  const { data: d, err, reload } = usePage(() => userApi.getMe())
  const [act, busy] = useAct(reload)
  const { renewToken } = useAuth()
  const [pw, setPw] = useState({ old: '', new: '' })
  if (!d) return <PageState err={err} retry={reload} />
  const u = d.user || {}

  // Đổi mật khẩu: token cũ hết hiệu lực, lưu ngay access_token mới máy chủ trả về.
  const change = async () => {
    const res = await userApi.changePassword(pw)
    if (res?.data?.access_token) renewToken(res.data.access_token)
    return res
  }

  return (
    <section className="stack">
      <PageHead title={u.name} sub={`${ROLE_LABEL[u.role] || u.role} · đăng nhập bằng ${u.username}`} />
      <form
        className="card stack narrow"
        onSubmit={(e) => {
          e.preventDefault()
          act('pw', change).then((j) => j && setPw({ old: '', new: '' }))
        }}
      >
        <h2>Đổi mật khẩu</h2>
        <label className="field">
          <span>Mật khẩu hiện tại</span>
          <input className="input" type="password" autoComplete="current-password" value={pw.old} onChange={(e) => setPw({ ...pw, old: e.target.value })} />
        </label>
        <label className="field">
          <span>Mật khẩu mới (từ 6 ký tự)</span>
          <input className="input" type="password" autoComplete="new-password" value={pw.new} onChange={(e) => setPw({ ...pw, new: e.target.value })} />
        </label>
        <Button type="submit" disabled={!pw.old || pw.new.length < 6} loading={busy === 'pw'}>
          Đổi mật khẩu
        </Button>
      </form>
    </section>
  )
}
