// Package province: 34 tỉnh, thành phố trực thuộc trung ương sau sắp xếp (Nghị quyết 202/2025/QH15, hiệu lực 1/7/2025).
// Gán tỉnh cho công trình theo toạ độ (ranh giới OCHA / HDX cod-ab-vnm 2025-09-25, CC BY-IGO, đã rút gọn ~200m),
// không có toạ độ thì theo địa chỉ, kể cả địa chỉ còn ghi tên 63 tỉnh cũ (vd "Bình Dương" -> TP. Hồ Chí Minh).
package province

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:embed provinces.geojson
var GeoJSON []byte

// Province: 1 tỉnh / thành mới. Code: mã OCHA (VN01 Hà Nội, VN79 TP. Hồ Chí Minh...).
type Province struct {
	Code   string   `json:"code"`
	Name   string   `json:"name"`
	Merged []string `json:"merged"` // tỉnh cũ đã nhập vào (trống: giữ nguyên)
}

// merged: tỉnh mới -> các tỉnh cũ nhập vào (Nghị quyết 202/2025/QH15). Tên mới trùng 1 tỉnh cũ thì tỉnh đó cũng có trong danh sách.
var merged = map[string][]string{
	"Tuyên Quang":     {"Tuyên Quang", "Hà Giang"},
	"Lào Cai":         {"Lào Cai", "Yên Bái"},
	"Thái Nguyên":     {"Thái Nguyên", "Bắc Kạn"},
	"Phú Thọ":         {"Phú Thọ", "Vĩnh Phúc", "Hòa Bình"},
	"Bắc Ninh":        {"Bắc Ninh", "Bắc Giang"},
	"Hưng Yên":        {"Hưng Yên", "Thái Bình"},
	"Hải Phòng":       {"Hải Phòng", "Hải Dương"},
	"Ninh Bình":       {"Ninh Bình", "Hà Nam", "Nam Định"},
	"Quảng Trị":       {"Quảng Trị", "Quảng Bình"},
	"Đà Nẵng":         {"Đà Nẵng", "Quảng Nam"},
	"Quảng Ngãi":      {"Quảng Ngãi", "Kon Tum"},
	"Gia Lai":         {"Gia Lai", "Bình Định"},
	"Khánh Hòa":       {"Khánh Hòa", "Ninh Thuận"},
	"Lâm Đồng":        {"Lâm Đồng", "Đắk Nông", "Bình Thuận"},
	"Đắk Lắk":         {"Đắk Lắk", "Phú Yên"},
	"TP. Hồ Chí Minh": {"Hồ Chí Minh", "Bình Dương", "Bà Rịa - Vũng Tàu"},
	"Đồng Nai":        {"Đồng Nai", "Bình Phước"},
	"Tây Ninh":        {"Tây Ninh", "Long An"},
	"Cần Thơ":         {"Cần Thơ", "Sóc Trăng", "Hậu Giang"},
	"Vĩnh Long":       {"Vĩnh Long", "Bến Tre", "Trà Vinh"},
	"Đồng Tháp":       {"Đồng Tháp", "Tiền Giang"},
	"Cà Mau":          {"Cà Mau", "Bạc Liêu"},
	"An Giang":        {"An Giang", "Kiên Giang"},
}

// Cách viết khác của tên tỉnh trong địa chỉ (Timemark, Google, người gõ).
var aliases = map[string]string{
	"tp hcm": "Hồ Chí Minh", "tphcm": "Hồ Chí Minh", "hcm": "Hồ Chí Minh", "sai gon": "Hồ Chí Minh", "ho chi minh city": "Hồ Chí Minh",
	"ba ria vung tau": "Bà Rịa - Vũng Tàu", "brvt": "Bà Rịa - Vũng Tàu", "vung tau": "Bà Rịa - Vũng Tàu",
	"thua thien hue": "Huế", "hue": "Huế", "dak nong": "Đắk Nông", "dac nong": "Đắk Nông", "dak lak": "Đắk Lắk", "daklak": "Đắk Lắk",
	"ha noi": "Hà Nội", "hanoi": "Hà Nội", "da nang": "Đà Nẵng", "danang": "Đà Nẵng", "hoa binh": "Hòa Bình", "khanh hoa": "Khánh Hòa",
}

type ring [][2]float64

type shape struct {
	p     *Province
	polys [][]ring // [đa giác][vòng 0 = viền ngoài, còn lại = lỗ]
	minX, minY, maxX, maxY float64
}

var (
	once   sync.Once
	shapes []shape
	all    []Province
	byKey  map[string]*Province // tên (bỏ dấu, thường) tỉnh mới hoặc cũ -> tỉnh mới
	keys   []string             // khoá dài trước (khớp "ba ria vung tau" trước "vung tau")
)

func load() {
	var fc struct {
		Features []struct {
			Properties struct{ Code, Name string } `json:"properties"`
			Geometry   struct {
				Coordinates [][]ring `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(GeoJSON, &fc); err != nil {
		panic("province: " + err.Error())
	}
	byKey = map[string]*Province{}
	for _, f := range fc.Features {
		p := &Province{Code: f.Properties.Code, Name: f.Properties.Name, Merged: merged[f.Properties.Name]}
		all = append(all, *p)
		s := shape{p: p, polys: f.Geometry.Coordinates, minX: 999, minY: 999, maxX: -999, maxY: -999}
		for _, poly := range s.polys {
			for _, c := range poly[0] {
				s.minX, s.maxX = min(s.minX, c[0]), max(s.maxX, c[0])
				s.minY, s.maxY = min(s.minY, c[1]), max(s.maxY, c[1])
			}
		}
		shapes = append(shapes, s)
		byKey[Key(strings.TrimPrefix(p.Name, "TP. "))] = p
		for _, old := range p.Merged {
			byKey[Key(old)] = p
		}
	}
	for a, old := range aliases {
		if p, ok := byKey[Key(old)]; ok {
			byKey[a] = p
		}
	}
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
}

// All: 34 tỉnh, thành (theo mã).
func All() []Province {
	once.Do(load)
	return all
}

// ByCode: tỉnh theo mã, nil nếu không có.
func ByCode(code string) *Province {
	once.Do(load)
	for i := range all {
		if all[i].Code == code {
			return &all[i]
		}
	}
	return nil
}

// At: tỉnh chứa toạ độ, nil nếu ngoài lãnh thổ (biển xa bờ...).
func At(lat, lng float64) *Province {
	once.Do(load)
	for i := range shapes {
		s := &shapes[i]
		if lng < s.minX || lng > s.maxX || lat < s.minY || lat > s.maxY {
			continue
		}
		for _, poly := range s.polys {
			if !inRing(poly[0], lng, lat) {
				continue
			}
			hole := false
			for _, h := range poly[1:] {
				if inRing(h, lng, lat) {
					hole = true
					break
				}
			}
			if !hole {
				return s.p
			}
		}
	}
	return nil
}

func inRing(r ring, x, y float64) bool {
	in := false
	for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
		xi, yi, xj, yj := r[i][0], r[i][1], r[j][0], r[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Key: bỏ dấu, chữ thường, chỉ chữ số + chữ cái cách nhau 1 dấu cách ("Bà Rịa - Vũng Tàu" -> "ba ria vung tau").
func Key(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, _ := transform.String(t, s)
	out = strings.NewReplacer("đ", "d", "Đ", "d").Replace(strings.ToLower(out))
	return strings.TrimSpace(nonAlnum.ReplaceAllString(out, " "))
}

// FromAddress: tỉnh mới theo địa chỉ (tên tỉnh mới hoặc cũ). Xét từ cuối địa chỉ lên (tỉnh thường ở cuối). Không thấy: nil.
func FromAddress(addr string) *Province {
	once.Do(load)
	parts := strings.Split(addr, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		k := " " + Key(parts[i]) + " "
		k = strings.NewReplacer(" tinh ", " ", " thanh pho ", " ", " tp ", " ").Replace(k)
		for _, key := range keys {
			if strings.Contains(k, " "+key+" ") {
				return byKey[key]
			}
		}
	}
	return nil
}

// Find: tỉnh theo toạ độ, không có thì theo địa chỉ.
func Find(lat, lng *float64, addr string) *Province {
	if lat != nil && lng != nil {
		if p := At(*lat, *lng); p != nil {
			return p
		}
	}
	return FromAddress(addr)
}
