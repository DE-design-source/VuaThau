package province

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// Danh sách 34 tỉnh / thành và 3.321 phường / xã / đặc khu sau sắp xếp đơn vị hành chính 2025
// (nguồn: provinces.open-api.vn/api/v2, lấy ngày 24/11/2026). Dùng cho ô Tỉnh / Phường của form Đăng dự án (app nhà thầu).
//
//go:embed wards.json
var WardsJSON []byte

type Division struct {
	Code  int      `json:"code"`
	Name  string   `json:"name"` // tên ngắn: "TP. Hồ Chí Minh", "Đồng Nai"
	Full  string   `json:"full"` // "Thành phố Hồ Chí Minh", "Tỉnh Đồng Nai"
	Wards []string `json:"wards"`
}

var (
	divOnce sync.Once
	divs    []Division
)

func Divisions() []Division {
	divOnce.Do(func() { _ = json.Unmarshal(WardsJSON, &divs) })
	return divs
}

// Ward: tìm tỉnh và phường đúng tên (không phân biệt hoa thường / dấu cách thừa). ok=false nếu không có.
func Ward(prov, ward string) (p, w string, ok bool) {
	pk, wk := Key(prov), Key(ward)
	for _, d := range Divisions() {
		if Key(d.Name) != pk && Key(d.Full) != pk {
			continue
		}
		for _, x := range d.Wards {
			if Key(x) == wk {
				return d.Name, x, true
			}
		}
		return d.Name, "", false
	}
	return "", "", false
}

// SameProvince: 2 tên cùng 1 tỉnh ("TP. Hồ Chí Minh" = "Thành phố Hồ Chí Minh" = "Hồ Chí Minh").
func SameProvince(a, b string) bool {
	norm := func(s string) string {
		k := Key(s)
		for _, p := range []string{"thanh pho ", "tp ", "tinh "} {
			k = strings.TrimPrefix(k, p)
		}
		return k
	}
	return a != "" && b != "" && norm(a) == norm(b)
}

// Join: địa chỉ đầy đủ "số nhà đường, phường, tỉnh" (bỏ phần trống).
func Join(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ", ")
}
