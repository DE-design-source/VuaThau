// Admin: hồ sơ chờ duyệt (/queue). Duyệt nhiều một lúc; trả lại / từ chối làm trong từng hồ sơ vì cần lý do.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import adminApi from '@/api/adminApi'
import { Button, Empty, HouseTable, PageHead, PageState } from '@/components/ui'

export default function QueuePage() {
  const { data: d, err, reload } = usePage(() => adminApi.queue())
  const [act, busy] = useAct(reload)
  const [sel, setSel] = useState(() => new Set())

  if (!d) return <PageState err={err} retry={reload} />
  const rows = d.rows || []
  const toggle = (id) =>
    setSel((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  const all = rows.length > 0 && sel.size === rows.length

  const approve = async () => {
    if (!window.confirm(`Duyệt ${sel.size} hồ sơ đã chọn?`)) return
    const j = await act('many', () => adminApi.approve([...sel]))
    if (j) setSel(new Set())
  }

  return (
    <section className="stack">
      <PageHead title="Hồ sơ chờ duyệt" sub={`${rows.length} hồ sơ đang chờ. Mở từng hồ sơ để xem ảnh, trả lại hoặc từ chối kèm lý do.`}>
        {rows.length > 0 && (
          <>
            <Link className="btn btn-secondary" to={`/houses/${rows[0].id}`}>
              Duyệt lần lượt từ #{rows[0].id}
            </Link>
            <Button onClick={approve} disabled={!sel.size} loading={busy === 'many'}>
              Duyệt {sel.size || ''} hồ sơ đã chọn
            </Button>
          </>
        )}
      </PageHead>
      {rows.length ? (
        <div className="card">
          <label className="check-all">
            <input type="checkbox" checked={all} onChange={() => setSel(all ? new Set() : new Set(rows.map((r) => r.id)))} /> Chọn tất cả
          </label>
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
