import { useEffect, useState } from 'react'
import ntClient from '@/api/ntClient'

// Hằng số chung với app (trạng thái, luật tiền, giai đoạn, ngân hàng), lấy 1 lần từ /app.json.
let cache = null
let pending = null

export function loadCfg() {
  if (cache) return Promise.resolve(cache)
  pending ??= ntClient.get('/app.json').then((c) => (cache = c)).finally(() => (pending = null))
  return pending
}

export default function useCfg() {
  const [cfg, setCfg] = useState(cache)
  useEffect(() => {
    if (!cache) loadCfg().then(setCfg, () => {})
  }, [])
  return cfg
}
