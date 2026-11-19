// Bản đồ cho sale (/sale/map), kiểu Airbnb: thanh tìm (Địa chỉ | Tỉnh thành | Cộng tác viên), lưới thẻ công trình
// có ảnh bên trái, bản đồ nhãn giai đoạn thi công bên phải. Sale thấy tên CTV để lọc; không thấy SĐT, tiền.
import { useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import saleApi from '@/api/saleApi'
import dezonApi from '@/api/dezonApi'
import { Kpi, PageState } from '@/components/ui'
import Explorer from '@/components/explore/Explorer'
import { siteTip, stageLabel } from '@/utils/siteTip'
import { gmapUrl, hsId, isStale, vnDate } from '@/utils/format'

export default function SaleMapPage() {
  const [sp, setSp] = useSearchParams()
  const cur = { province: sp.get('province') || '', ctv: sp.get('ctv') || '' }
  const { data: d, err, busy, reload } = usePage(() => saleApi.map(cur), [cur.province, cur.ctv])
  const dz = usePage(() => dezonApi.projects({ province: cur.province }), [cur.province]) // lỗi: bỏ qua, chỉ thiếu lớp Dezon
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const provinces = d.provinces || []
  const ctvs = d.ctvs || []
  const stale = d.stale_days || 30
  const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
  const set = (patch) => {
    const next = { ...cur, ...patch }
    setSp(Object.fromEntries(Object.entries(next).filter(([, v]) => v)))
  }
  const where = (p) => name[p.province] || 'chưa rõ tỉnh'

  return (
    <Explorer
      points={points}
      provinces={provinces}
      staleDays={stale}
      busy={busy}
      dezon={dz.data?.rows}
      fitKey={`${cur.province}|${cur.ctv}`}
      selectedProvince={cur.province}
      onProvince={(code) => set({ province: code === cur.province ? '' : code })}
      selects={[
        {
          key: 'province',
          label: 'Tỉnh thành',
          value: cur.province,
          onChange: (v) => set({ province: v }),
          options: [
            { value: '', label: 'Cả nước' },
            ...[...provinces].sort((a, b) => a.name.localeCompare(b.name, 'vi')).map((p) => ({ value: p.code, label: `${p.name} (${p.total})` })),
          ],
        },
        {
          key: 'ctv',
          label: 'Cộng tác viên',
          value: cur.ctv,
          onChange: (v) => set({ ctv: v }),
          options: [{ value: '', label: 'Tất cả' }, ...ctvs.map((c) => ({ value: String(c.id), label: `${c.name} (${c.total})` }))],
        },
      ]}
      pop={(p) =>
        siteTip(p, { staleDays: stale, prov: name[p.province] }) +
        `<div class="site-pop-actions"><a href="/sale/projects/${p.id}" data-to="/sale/projects/${p.id}">Xem dự án</a>` +
        `<a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`
      }
      card={(p) => {
        const old = isStale(p.last_at || p.at, stale)
        return {
          to: `/sale/projects/${p.id}`,
          badge: old ? 'Cần cập nhật' : null,
          title: `${stageLabel(p.stage)} tại ${where(p)}`,
          right: <span className="mono">{hsId(p.id)}</span>,
          lines: [p.address || '(chưa có địa chỉ)', p.contractor && `Nhà thầu: ${p.contractor}`, p.ctv_name && `CTV: ${p.ctv_name}`],
          when: (
            <>
              <u>Chụp lần cuối {vnDate(p.last_at || p.at)}</u>
              {old && <b className="text-ok"> · cần cập nhật</b>}
            </>
          ),
        }
      }}
      footer={
        <div className="kpis ex-kpis">
          <Kpi label="Công trình" value={points.length} sub={[name[cur.province], ctvs.find((c) => String(c.id) === cur.ctv)?.name].filter(Boolean).join(' · ') || 'cả nước'} />
          <Kpi label="Cần cập nhật" value={points.filter((p) => isStale(p.last_at || p.at, stale)).length} sub={`quá ${stale} ngày`} />
          <Kpi label="Tỉnh, thành có công trình" value={`${provinces.filter((p) => p.total > 0).length}/34`} />
          <Kpi label="Cộng tác viên đã chụp" value={ctvs.length} />
        </div>
      }
    />
  )
}
