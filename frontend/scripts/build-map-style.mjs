// Tạo public/map-style.json: nền bản đồ kiểu Airbnb cho web và app CTV (cùng một file, máy chủ phục vụ ở /map-style.json).
// Lấy kiểu "Liberty" của OpenFreeMap (miễn phí, dùng kinh doanh được, không cần key) rồi tô lại:
// đất be ấm, công viên xanh nhạt, nước xanh dương nhẹ, đường trắng viền xám, đường lớn vàng nhạt, chữ xám;
// bỏ toà nhà 3D, số nhà, "Khu phố 1, 2, 3...", phần lớn biểu tượng địa điểm; tên gốc tiếng Việt.
// Chạy lại khi muốn đổi màu:  node scripts/build-map-style.mjs
import { writeFileSync } from 'node:fs'

const SRC = 'https://tiles.openfreemap.org/styles/liberty'
const C = {
  land: '#f4f2ec', urban: '#efece4', park: '#d8ecc9', wood: '#cfe5bd', water: '#a9d3f0', waterLine: '#8fc3e8',
  building: '#e6e2d9', buildingLine: '#d9d4c8', road: '#ffffff', roadCase: '#dcd8cf', major: '#fdf3c8', majorCase: '#e8d79a',
  rail: '#d2cec4', text: '#5e5e5e', textMinor: '#8a8a8a', textPlace: '#9a958c', halo: '#ffffff', boundary: '#b9b3a6',
}
const keepPoi = /^(poi_r1|airport)/ // giữ rất ít biểu tượng: địa điểm lớn, sân bay

const st = await (await fetch(SRC)).json()
const layers = []
for (const l of st.layers) {
  const id = l.id
  const sl = l['source-layer'] || ''
  const p = (l.paint = l.paint || {})
  const lay = (l.layout = l.layout || {})
  if (l.type === 'background') p['background-color'] = C.land
  else if (l.type === 'fill-extrusion') continue // toà nhà 3D: bỏ
  else if (sl === 'water' && l.type === 'fill') p['fill-color'] = C.water
  else if (sl === 'waterway' && l.type === 'line') p['line-color'] = C.waterLine
  else if ((sl === 'landcover' || sl === 'landuse' || sl === 'park') && l.type === 'fill') {
    if (/wood|forest/.test(id)) p['fill-color'] = C.wood
    else if (/park|grass|garden|pitch|cemetery|golf|farmland|meadow|scrub|landcover/.test(id) || sl === 'park') p['fill-color'] = C.park
    else p['fill-color'] = C.urban
    p['fill-opacity'] = 0.9
    delete p['fill-pattern']
  } else if (sl === 'building' && l.type === 'fill') {
    p['fill-color'] = C.building
    p['fill-outline-color'] = C.buildingLine
  } else if (sl === 'transportation' && l.type === 'line') {
    const casing = /casing/.test(id)
    const major = /motorway|trunk/.test(id)
    if (/rail|transit/.test(id)) p['line-color'] = C.rail
    else p['line-color'] = casing ? (major ? C.majorCase : C.roadCase) : major ? C.major : C.road
    delete p['line-dasharray']
    if (/path|track|pedestrian|bridge_path/.test(id)) continue
  } else if (sl === 'boundary') p['line-color'] = C.boundary
  else if (l.type === 'symbol') {
    if (sl === 'poi' && !keepPoi.test(id)) continue
    if (/housenumber|aerodrome_label|highway-shield|road_shield|oneway/.test(id)) continue
    p['text-color'] = sl === 'transportation_name' || /road|street/.test(id) ? C.textMinor : C.text
    p['text-halo-color'] = C.halo
    p['text-halo-width'] = 1.4
    if (lay['text-field']) lay['text-field'] = ['coalesce', ['get', 'name'], ['get', 'name:latin'], ['get', 'name_en']]
    if (sl === 'place') {
      // bỏ "Khu phố 1, 2, 3..." (rối): lọc theo hạng địa danh, không dùng biểu thức chuỗi (app đọc được)
      if (id === 'label_other') l.filter = ['match', ['get', 'class'], ['suburb', 'quarter'], true, false] // giữ tên phường, bỏ khu phố / thôn
      if (!/city|capital|country|state|town/.test(id)) {
        lay['text-transform'] = 'uppercase'
        lay['text-letter-spacing'] = 0.12
        p['text-color'] = C.textPlace
      }
    }
  }
  layers.push(l)
}
st.layers = layers
st.name = 'Vua Thầu Map (kiểu Airbnb, dựa trên OpenFreeMap Liberty)'
const out = new URL('../public/map-style.json', import.meta.url)
writeFileSync(out, JSON.stringify(st))
console.log('ok', layers.length, 'lớp ->', out.pathname)
