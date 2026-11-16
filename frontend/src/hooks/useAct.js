import { useCallback, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import ntClient from '@/api/ntClient'
import { toast } from '@/services/toast'

/**
 * Gửi thao tác (POST) lên máy chủ như bấm nút trên web cũ: báo msgs, rồi đi tới url máy chủ trả về
 * (trùng trang đang xem thì tải lại). Route của web này trùng đường dẫn máy chủ nên url dùng thẳng được.
 */
export default function useAct(reload) {
  const [busy, setBusy] = useState('')
  const navigate = useNavigate()
  const { pathname, search } = useLocation()

  const act = useCallback(
    async (what, path, body, { stay = false } = {}) => {
      setBusy(what)
      try {
        const j = await ntClient.post(path, body)
        toast(j.msgs)
        const url = (j.url || '').split('#')[0]
        if (!stay && url && url !== pathname && url !== pathname + search && url !== '/login') navigate(url)
        else reload?.()
        return j
      } catch (e) {
        toast(e.message)
        return null
      } finally {
        setBusy('')
      }
    },
    [navigate, pathname, search, reload],
  )
  return [act, busy]
}
