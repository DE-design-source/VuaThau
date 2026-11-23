// Nhà thầu (/contractors, góp ý 23/11/2026, theo thiết kế "Danh sách nhà thầu"): 2 nguồn gộp theo SĐT
//   - đăng ký qua form Lark "Form Đăng ký nhà thầu - Vua Thầu" (tự đồng bộ 15 phút / lần; nút Thêm nhà thầu mở form);
//   - trích từ ảnh CTV chụp công trình (tên, SĐT trên bảng hiệu).
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import useAct from '@/hooks/useAct'
import contractorApi from '@/api/contractorApi'
import { Button, PageHead, PageState } from '@/components/ui'
import { Pill as FPill } from '@/components/explore/layers'
import { facet } from '@/utils/mapLayers'
import { initials, vnDate } from '@/utils/format'

const fold = (s) =>
  (s || '')
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/đ/g, 'd')
    .replace(/Đ/g, 'd')
    .toLowerCase()

const NO_F = { role: '', area: '', exp: '', value: '', status: '' }
const TONE = { 'Đã xác minh': 'ok', 'Chưa xác minh': 'draft', 'Từ ảnh CTV': 'wait' }
const PAGE = 50

/** Tên tháng tham gia: 03/2025 */
const month = (s) => vnDate(s).slice(3)

export default function ContractorListPage() {
  const { data: d, err, reload } = usePage(() => contractorApi.list())
  const [act, busy] = useAct(reload)
  const [q, setQ] = useState('')
  const [f, setF] = useState(NO_F)
  const [n, setN] = useState(PAGE)
  const nav = useNavigate()
  const all = useMemo(() => d?.rows || [], [d])
  // khu vực: tách theo dấu phẩy (form ghi 1 tỉnh, ảnh CTV có thể nhiều tỉnh)
  const areaOf = (r) => (r.areas || '').split(',').map((x) => x.trim()).filter(Boolean)
  const opts = useMemo(
    () => ({
      role: facet(all, (r) => r.role, 'Chưa rõ'),
      exp: facet(all, (r) => r.exp, 'Chưa rõ'),
      value: facet(all, (r) => r.value_range, 'Chưa rõ'),
      status: facet(all, (r) => r.status, ''),
      area: facet(
        all.flatMap((r) => areaOf(r).map((a) => ({ a }))),
        (x) => x.a,
        '',
      ),
    }),
    [all],
  )
  if (!d) return <PageState err={err} retry={reload} />
  const fq = fold(q.trim())
  const digits = q.replace(/\D/g, '')
  const eq = (v, want) => !want || (v || '') === (want === '-' ? '' : want)
  const rows = all.filter(
    (r) =>
      eq(r.role, f.role) &&
      eq(r.exp, f.exp) &&
      eq(r.value_range, f.value) &&
      eq(r.status, f.status) &&
      (!f.area || areaOf(r).includes(f.area)) &&
      (!fq || fold(`${r.name} ${r.areas} ${r.role}`).includes(fq) || (digits.length >= 4 && (r.phone || '').replace(/\D/g, '').includes(digits))),
  )
  const any = q || Object.entries(f).some(([k, v]) => v !== NO_F[k])
  const reg = all.filter((r) => r.registered).length

  return (
    <section className="stack">
      <PageHead
        title="Nhà thầu"
        sub={`${all.length} nhà thầu · ${reg} đăng ký qua form Vua Thầu, ${all.length - reg} trích từ ảnh CTV chụp công trình.`}
      >
        <Button variant="ghost" loading={busy === 'sync'} onClick={() => act('sync', () => contractorApi.sync())}>
          Đồng bộ form
        </Button>
        {d.form_url && (
          <a className="btn nt-add" href={d.form_url} target="_blank" rel="noreferrer">
            + Thêm nhà thầu
          </a>
        )}
      </PageHead>
      <div className="card nt-box">
        <input className="input nt-search" placeholder="Tên công ty, người đại diện, SĐT, khu vực" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="ex-filters nt-filters">
          <FPill label="Vai trò" value={f.role} options={opts.role} onChange={(v) => setF({ ...f, role: v })} />
          <FPill label="Khu vực" value={f.area} options={opts.area} onChange={(v) => setF({ ...f, area: v })} />
          <FPill label="Kinh nghiệm" value={f.exp} options={opts.exp} onChange={(v) => setF({ ...f, exp: v })} />
          <FPill label="Giá trị CT" value={f.value} options={opts.value} onChange={(v) => setF({ ...f, value: v })} />
          <FPill label="Trạng thái" value={f.status} options={opts.status} onChange={(v) => setF({ ...f, status: v })} />
        </div>
        <p className="muted nt-count">
          Hiển thị <b>{Math.min(n, rows.length)}</b> / {rows.length} nhà thầu
          {any && (
            <button
              type="button"
              className="cus-clear"
              onClick={() => {
                setQ('')
                setF(NO_F)
              }}
            >
              Xoá bộ lọc
            </button>
          )}
        </p>
        <div className="table-wrap">
          <table className="table nt-table">
            <thead>
              <tr>
                <th>Nhà thầu</th>
                <th>Vai trò</th>
                <th>Khu vực thi công</th>
                <th>Kinh nghiệm</th>
                <th>Giá trị CT</th>
                <th className="num">Công trình</th>
                <th>Trạng thái</th>
                <th>Cập nhật</th>
              </tr>
            </thead>
            <tbody>
              {rows.slice(0, n).map((r) => (
                <tr key={r.key} className="nt-row" onClick={() => nav(`/contractors/${encodeURIComponent(r.key)}`)}>
                  <td>
                    <span className="nt-who">
                      <span className={`nt-av ${r.status === 'Đã xác minh' ? 'on' : ''}`}>{initials(r.name)}</span>
                      <span>
                        <b className="nt-name">{r.name || r.phone || 'Chưa rõ tên'}</b>
                        <small className="muted">
                          {r.kind || (r.registered ? 'Đăng ký' : 'Từ ảnh CTV')} · {r.registered ? `Tham gia ${month(r.joined)}` : `Chụp từ ${vnDate(r.joined)}`}
                        </small>
                      </span>
                    </span>
                  </td>
                  <td>{r.role || <span className="muted">—</span>}</td>
                  <td className="nt-areas">{r.areas || <span className="muted">—</span>}</td>
                  <td>{r.exp || <span className="muted">—</span>}</td>
                  <td>{r.value_range || <span className="muted">—</span>}</td>
                  <td className="num">
                    <b>{r.total}</b>
                  </td>
                  <td>
                    <span className={`badge tone-${TONE[r.status] || 'draft'}`}>{r.status}</span>
                  </td>
                  <td className="nowrap">{vnDate(r.updated)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!rows.length && <p className="muted">Không có nhà thầu phù hợp bộ lọc.</p>}
        {rows.length > n && (
          <div className="ex-more">
            <button type="button" className="btn btn-secondary" onClick={() => setN(n + PAGE)}>
              Xem thêm {Math.min(PAGE, rows.length - n)} nhà thầu
            </button>
          </div>
        )}
      </div>
    </section>
  )
}
