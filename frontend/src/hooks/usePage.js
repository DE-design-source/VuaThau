import { useCallback, useEffect, useEffectEvent, useState } from 'react'

/**
 * Dữ liệu 1 trang từ API. fetcher() trả promise envelope {success, data}; deps: giá trị làm tải lại (vd id, bộ lọc).
 * Trả { data, err, busy, reload }. Đổi deps: dữ liệu cũ vẫn hiện tới khi có dữ liệu mới, busy báo đang tải.
 */
export default function usePage(fetcher, deps = []) {
  const key = JSON.stringify(deps)
  const [state, setState] = useState({ data: null, err: '', key: null })
  const [n, setN] = useState(0)
  const run = useEffectEvent(() => fetcher())

  useEffect(() => {
    let dead = false
    const tag = `${key}#${n}`
    run().then(
      (res) => !dead && setState({ data: res?.data ?? null, err: '', key: tag }),
      (e) => !dead && setState((s) => ({ ...s, err: e.message, key: tag })),
    )
    return () => {
      dead = true
    }
  }, [key, n])

  const reload = useCallback(() => setN((k) => k + 1), [])
  return { data: state.data, err: state.err, busy: state.key !== `${key}#${n}`, reload }
}
