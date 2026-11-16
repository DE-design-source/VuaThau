// provprep: thu gọn ranh giới 34 tỉnh, thành (sau sáp nhập 1/7/2025) từ dữ liệu OCHA / HDX (cod-ab-vnm, CC BY-IGO)
// thành file nhỏ dùng cho backend (gán tỉnh theo toạ độ) và bản đồ web.
//
//	go run ./cmd/provprep -in vnm_admin1.geojson -out internal/province/provinces.geojson [-tol 0.003]
//
// Nguồn: https://data.humdata.org/dataset/cod-ab-vnm (vnm_admin_boundaries.geojson.zip, lớp admin1).
// Giữ mọi đảo (kể cả Hoàng Sa, Trường Sa): vòng quá nhỏ sau khi rút gọn thì giữ bản thô vài điểm.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
)

type feature struct {
	Properties map[string]any `json:"properties"`
	Geometry   struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

type pt [2]float64

func main() {
	in := flag.String("in", "", "vnm_admin1.geojson của OCHA")
	out := flag.String("out", "internal/province/provinces.geojson", "file ra")
	tol := flag.Float64("tol", 0.003, "sai số rút gọn (độ, ~0.003 = 300m)")
	flag.Parse()
	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	var src struct {
		Features []feature `json:"features"`
	}
	if err := json.Unmarshal(raw, &src); err != nil {
		log.Fatal(err)
	}
	type outF struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
		Geometry   map[string]any `json:"geometry"`
	}
	res := struct {
		Type        string `json:"type"`
		Attribution string `json:"attribution"`
		Features    []outF `json:"features"`
	}{Type: "FeatureCollection", Attribution: "Ranh giới hành chính: OCHA / HDX cod-ab-vnm (2025-09-25), CC BY-IGO"}
	for _, f := range src.Features {
		var polys [][][]pt
		switch f.Geometry.Type {
		case "Polygon":
			var p [][]pt
			if err := json.Unmarshal(f.Geometry.Coordinates, &p); err != nil {
				log.Fatal(err)
			}
			polys = [][][]pt{p}
		case "MultiPolygon":
			if err := json.Unmarshal(f.Geometry.Coordinates, &polys); err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatalf("hình lạ: %s", f.Geometry.Type)
		}
		var outPolys [][][]pt
		for _, p := range polys {
			var rings [][]pt
			for i, r := range p {
				s := simplifyRing(r, *tol)
				if s == nil {
					if i > 0 { // lỗ quá nhỏ: bỏ
						continue
					}
					s = coarse(r)
				}
				rings = append(rings, s)
			}
			if len(rings) > 0 {
				outPolys = append(outPolys, rings)
			}
		}
		code, _ := f.Properties["adm1_pcode"].(string)
		name, _ := f.Properties["adm1_name1"].(string)
		res.Features = append(res.Features, outF{Type: "Feature",
			Properties: map[string]any{"code": code, "name": name},
			Geometry:   map[string]any{"type": "MultiPolygon", "coordinates": round(outPolys)}})
	}
	sort.Slice(res.Features, func(i, j int) bool {
		return res.Features[i].Properties["code"].(string) < res.Features[j].Properties["code"].(string)
	})
	b, err := json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d tỉnh, %d KB -> %s\n", len(res.Features), len(b)/1024, *out)
}

// simplifyRing: Douglas–Peucker; nil nếu còn dưới 4 điểm.
func simplifyRing(r []pt, tol float64) []pt {
	if len(r) < 4 {
		return nil
	}
	keep := make([]bool, len(r))
	keep[0], keep[len(r)-1] = true, true
	dp(r, 0, len(r)-1, tol, keep)
	out := []pt{}
	for i, k := range keep {
		if k {
			out = append(out, r[i])
		}
	}
	if len(out) < 4 {
		return nil
	}
	return out
}

func dp(r []pt, a, b int, tol float64, keep []bool) {
	if b <= a+1 {
		return
	}
	best, at := -1.0, -1
	for i := a + 1; i < b; i++ {
		if d := segDist(r[i], r[a], r[b]); d > best {
			best, at = d, i
		}
	}
	if best > tol {
		keep[at] = true
		dp(r, a, at, tol, keep)
		dp(r, at, b, tol, keep)
	}
}

func segDist(p, a, b pt) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	if dx == 0 && dy == 0 {
		return math.Hypot(p[0]-a[0], p[1]-a[1])
	}
	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p[0]-(a[0]+t*dx), p[1]-(a[1]+t*dy))
}

// coarse: đảo nhỏ, giữ tối đa ~8 điểm đều nhau (vẫn là vòng khép kín).
func coarse(r []pt) []pt {
	if len(r) <= 9 {
		return r
	}
	step := len(r) / 8
	out := []pt{}
	for i := 0; i < len(r)-1; i += step {
		out = append(out, r[i])
	}
	return append(out, r[0])
}

func round(polys [][][]pt) [][][][2]float64 {
	q := func(v float64) float64 { return math.Round(v*1e4) / 1e4 }
	out := make([][][][2]float64, len(polys))
	for i, p := range polys {
		out[i] = make([][][2]float64, len(p))
		for j, r := range p {
			out[i][j] = make([][2]float64, len(r))
			for k, c := range r {
				out[i][j][k] = [2]float64{q(c[0]), q(c[1])}
			}
		}
	}
	return out
}
