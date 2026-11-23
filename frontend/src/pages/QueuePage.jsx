// Admin: hồ sơ chờ duyệt (/queue). Duyệt nhiều một lúc; trả lại / từ chối làm trong từng hồ sơ vì cần lý do.
// Lọc (tìm mã / địa chỉ / nhà thầu / CTV, nguồn App / Messenger, ngày tạo từ - đến) và sắp xếp theo ngày tạo
// (mặc định mới nhất trước). Bộ lọc nằm trên URL: ?q=&src=&from=&to=&sort=cu
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import adminApi from '@/api/adminApi'
import { Button, Empty, HouseTable, PageHead, PageState, hsId } from '@/components/ui'

/** Ngày (giờ Việt Nam) dạng YYYY-MM-DD, để so với ô chọn ngày. */
const vnDay = (s) => (s ? new Date(s).toLocaleDateString('en-CA', { timeZone: 'Asia/Ho_Chi_Minh' }) : '')
const norm = (s) =>
  (s || '')
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/đ/g, 'd')

export default function QueuePage() {
  const { data: d, err, reload } = usePage(() => adminApi.queue())
  const [act, busy] = useAct(reload)
  const [sel, setSel] = useState(() => new Set())
  const [sp, setSp] = useSearchParams()
  const f = { q: sp.get('q') || '', src: sp.get('src') || '', from: sp.get('from') || '', to: sp.get('to') || '', sort: sp.get('sort') || 'moi' }
  const [q, setQ] = useState(f.q)
  const set = (chg) => {
    const next = { ...f, ...chg }
    if (next.sort === 'moi') delete next.sort
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
    setSel(new Set())
  }

  const all = useMemo(() => d?.rows || [], [d])
  const rows = useMemo(() => {
    const k = norm(f.q.trim()).replace(/^hs-0*/, '')
    const out = all.filter((r) => {
      if (f.src === 'app' && r.source === 'inbox') return false
      if (f.src === 'inbox' && r.source !== 'inbox') return false
      const day = vnDay(r.created_at)
      if (f.from && day < f.from) return false
      if (f.to && day > f.to) return false
      if (!k) return true
      return String(r.id) === k || [r.address, r.ctv_name, r.contractor_name, r.contractor_phone].some((x) => norm(x).includes(k))
    })
    const t = (r) => new Date(r.created_at).getTime() || 0
    return out.sort((a, b) => (f.sort === 'cu' ? t(a) - t(b) || a.id - b.id : t(b) - t(a) || b.id - a.id))
  }, [all, f.q, f.src, f.from, f.to, f.sort])

  if (!d) return <PageState err={err} retry={reload} />
  const toggle = (id) =>
    setSel((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  const allOn = rows.length > 0 && rows.every((r) => sel.has(r.id))
  const filtered = f.q || f.src || f.from || f.to

  const approve = async () => {
    if (!window.confirm(`Duyệt ${sel.size} hồ sơ đã chọn?`)) return
    const j = await act('many', () => adminApi.approve([...sel]))
    if (j) setSel(new Set())
  }

  return (
    <section className="stack">
      <PageHead title="Duyệt công trình" sub={`${all.length} hồ sơ đang chờ. Mở từng hồ sơ để xem ảnh, trả lại hoặc từ chối kèm lý do.`}>
        {rows.length > 0 && (
          <>
            <Link className="btn btn-secondary" to={`/houses/${rows[0].id}`}>
              Duyệt lần lượt từ {hsId(rows[0].id)}
            </Link>
            <Button onClick={approve} disabled={!sel.size} loading={busy === 'many'}>
              Duyệt {sel.size || ''} hồ sơ đã chọn
            </Button>
          </>
        )}
      </PageHead>

      {all.length > 0 && (
        <form
          className="card filters"
          onSubmit={(e) => {
            e.preventDefault()
            set({ q: q.trim() })
          }}
        >
          <input className="input" placeholder="Tìm mã hồ sơ, địa chỉ, nhà thầu, tên CTV" value={q} onChange={(e) => setQ(e.target.value)} />
          <select className="input" style={{ flex: '0 1 170px' }} value={f.src} onChange={(e) => set({ src: e.target.value })} aria-label="Nguồn hồ sơ">
            <option value="">Mọi nguồn</option>
            <option value="app">App CTV</option>
            <option value="inbox">Messenger</option>
          </select>
          <label className="inline">
            Tạo từ <input className="input" type="date" value={f.from} onChange={(e) => set({ from: e.target.value })} />
          </label>
          <label className="inline">
            đến <input className="input" type="date" value={f.to} onChange={(e) => set({ to: e.target.value })} />
          </label>
          <select className="input" style={{ flex: '0 1 190px' }} value={f.sort} onChange={(e) => set({ sort: e.target.value })} aria-label="Sắp xếp">
            <option value="moi">Ngày tạo: mới nhất trước</option>
            <option value="cu">Ngày tạo: cũ nhất trước</option>
          </select>
          <Button type="submit">Tìm</Button>
          {filtered && (
            <Button
              variant="ghost"
              onClick={() => {
                setQ('')
                set({ q: '', src: '', from: '', to: '' })
              }}
            >
              Xoá lọc
            </Button>
          )}
        </form>
      )}

      {all.length ? (
        <div className="card">
          <div className="row-between">
            <label className="check-all">
              <input type="checkbox" checked={allOn} disabled={!rows.length} onChange={() => setSel(allOn ? new Set() : new Set(rows.map((r) => r.id)))} /> Chọn tất cả
              {filtered ? ' (đang lọc)' : ''}
            </label>
            {filtered && (
              <span className="muted">
                {rows.length}/{all.length} hồ sơ
              </span>
            )}
          </div>
          <HouseTable rows={rows} selectable selected={sel} onToggle={toggle} />
        </div>
      ) : (
        <div className="card">
          <Empty title="Đã xử lý hết hồ sơ chờ duyệt" text="Hồ sơ mới CTV gửi từ app sẽ hiện ở đây." />
        </div>
      )}
    </section>
  )
}
