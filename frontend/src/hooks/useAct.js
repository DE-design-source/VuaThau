import { useCallback, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from '@/services/toast'

/**
 * Gửi thao tác ghi lên API: báo data.message, rồi đi tới trang `go` (nếu có) hoặc tải lại trang đang xem.
 *   act(what, () => api.call(...), { go: (data) => '/batches/' + data.batch_id })
 * what: tên nút đang bận (busy). go: chuỗi hoặc hàm (data) => chuỗi | falsy. Trả data, lỗi trả null.
 */
export default function useAct(reload) {
  const [busy, setBusy] = useState('')
  const navigate = useNavigate()
  const { pathname } = useLocation()

  const act = useCallback(
    async (what, call, { go } = {}) => {
      setBusy(what)
      try {
        const res = await call()
        const data = res?.data ?? {}
        toast(data.message)
        const to = typeof go === 'function' ? go(data) : go
        if (to && to !== pathname) navigate(to)
        else reload?.()
        return data
      } catch (e) {
        toast(e.message)
        return null
      } finally {
        setBusy('')
      }
    },
    [navigate, pathname, reload],
  )
  return [act, busy]
}
