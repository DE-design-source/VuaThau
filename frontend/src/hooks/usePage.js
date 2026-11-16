import { useCallback, useEffect, useState } from 'react'
import ntClient from '@/api/ntClient'
import { toast } from '@/services/toast'

/** Dữ liệu 1 trang từ máy chủ Nghiệm thu. reload() tải lại (sau khi thao tác). */
export default function usePage(path) {
  const [state, setState] = useState({ data: null, err: '', path: null })
  const [key, setKey] = useState(0)

  useEffect(() => {
    let dead = false
    ntClient.get(path).then(
      (j) => {
        if (dead) return
        toast(j.msgs)
        setState({ data: j, err: '', path })
      },
      (e) => {
        if (!dead) setState((s) => ({ ...s, err: e.message, path }))
      },
    )
    return () => {
      dead = true
    }
  }, [path, key])

  const reload = useCallback(() => setKey((k) => k + 1), [])
  // Đổi trang (vd lọc khác): dữ liệu cũ vẫn hiện tới khi có dữ liệu mới, busy báo đang tải
  return { data: state.data, err: state.err, busy: state.path !== path, reload }
}
