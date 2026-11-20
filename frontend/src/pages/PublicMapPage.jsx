// Bản đồ công khai (/ban-do, góp ý 20/11/2026): người chưa đăng nhập xem công trình như sale nhưng không tên CTV,
// không SĐT, tiền, trạng thái duyệt. Ảnh đại diện đã che SĐT, tải qua link có chữ ký (không cần đăng nhập).
import { Link, useSearchParams } from 'react-router-dom'
import usePage from '@/hooks/usePage'
import publicApi from '@/api/publicApi'
import { Kpi, PageState } from '@/components/ui'
import Explorer from '@/components/explore/Explorer'
import { cardProps, siteTip } from '@/utils/siteTip'
import { gmapUrl, isStale } from '@/utils/format'

export default function PublicMapPage() {
  const [sp, setSp] = useSearchParams()
  const province = sp.get('province') || ''
  const { data: d, err, busy, reload } = usePage(() => publicApi.map({ province }), [province])
  if (!d) return <PageState err={err} retry={reload} />
  const points = d.points || []
  const provinces = d.provinces || []
  const stale = d.stale_days || 30
  const name = Object.fromEntries(provinces.map((p) => [p.code, p.name]))
  const set = (v) => setSp(v ? { province: v } : {})

  return (
    <Explorer
      points={points}
      provinces={provinces}
      staleDays={stale}
      busy={busy}
      fitKey={province}
      selectedProvince={province}
      onProvince={(code) => set(code === province ? '' : code)}
      selects={[
        {
          key: 'province',
          label: 'Tỉnh thành',
          value: province,
          onChange: set,
          options: [
            { value: '', label: 'Cả nước' },
            ...[...provinces].sort((a, b) => a.name.localeCompare(b.name, 'vi')).map((p) => ({ value: p.code, label: `${p.name} (${p.total})` })),
          ],
        },
      ]}
      pop={(p) =>
        siteTip(p, { staleDays: stale, prov: name[p.province] }) +
        `<div class="site-pop-actions"><a href="${gmapUrl(p.lat, p.lng)}" target="_blank" rel="noreferrer">Google Maps</a></div>`
      }
      card={(p) => ({ ...cardProps(p, { prov: name[p.province], staleDays: stale }), href: gmapUrl(p.lat, p.lng), img: p.cover_url })}
      footer={
        <>
          <div className="kpis ex-kpis">
            <Kpi label="Công trình" value={points.length} sub={name[province] || 'cả nước'} />
            <Kpi label="Cần cập nhật" value={points.filter((p) => isStale(p.last_at || p.at, stale)).length} sub={`quá ${stale} ngày`} />
            <Kpi label="Tỉnh, thành có công trình" value={`${provinces.filter((p) => p.total > 0).length}/34`} />
          </div>
          <p className="muted">
            Xem thêm hồ sơ, nhà thầu, báo cáo: <Link to="/login">đăng nhập</Link>.
          </p>
        </>
      }
    />
  )
}
