import { useEffect, useState } from 'react'
import configApi from '@/api/configApi'

// Hằng số chung với app (trạng thái, luật tiền, giai đoạn, lý do mẫu), lấy 1 lần từ GET /config.
let cache = null
let pending = null
const subs = new Set()

export function loadCfg() {
  if (cache) return Promise.resolve(cache)
  pending ??= configApi
    .get()
    .then((r) => {
      cache = r.data
      subs.forEach((f) => f(cache))
      return cache
    })
    .finally(() => (pending = null))
  return pending
}

/** Bỏ bộ nhớ đệm và tải lại (vd admin vừa lưu Cấu hình). */
export function reloadCfg() {
  cache = null
  return loadCfg().catch(() => {})
}

export default function useCfg() {
  const [cfg, setCfg] = useState(cache)
  useEffect(() => {
    subs.add(setCfg)
    if (!cache) loadCfg().catch(() => {})
    return () => {
      subs.delete(setCfg)
    }
  }, [])
  return cfg
}
