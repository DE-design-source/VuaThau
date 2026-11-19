// Package geo: địa chỉ của toạ độ GPS để in lên ảnh camera. Có key Google thì dùng Google Geocoding,
// không thì OpenStreetMap Nominatim (miễn phí, tối đa 1 lượt/giây, bắt buộc User-Agent nêu tên app).
package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const userAgent = "VuaThau-NghiemThu/2.0 (+https://map.vuathau.com.vn)"

type Geocoder struct {
	googleKey string
	http      *http.Client

	mu     sync.Mutex
	cache  map[string]string  // toạ độ làm tròn ~11m -> địa chỉ
	search map[string][]Place // tìm địa điểm: câu tìm -> kết quả
	order  []string

	nomiMu   sync.Mutex
	nomiLast time.Time
}

func New(googleKey string) *Geocoder {
	return &Geocoder{googleKey: googleKey, http: &http.Client{Timeout: 6 * time.Second}, cache: map[string]string{}}
}

const cacheMax = 4096

// Address: địa chỉ của toạ độ. Lỗi tra cứu: trả lỗi (không lưu đệm).
func (g *Geocoder) Address(ctx context.Context, lat, lng float64) (string, error) {
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return "", fmt.Errorf("toạ độ không hợp lệ")
	}
	key := fmt.Sprintf("%.4f,%.4f", lat, lng)
	g.mu.Lock()
	if a, ok := g.cache[key]; ok {
		g.mu.Unlock()
		return a, nil
	}
	g.mu.Unlock()

	var a string
	var err error
	if g.googleKey != "" {
		a, err = g.google(ctx, key)
	} else {
		a, err = g.osm(ctx, key)
	}
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	if _, ok := g.cache[key]; !ok {
		g.cache[key] = a
		g.order = append(g.order, key)
		if len(g.order) > cacheMax {
			delete(g.cache, g.order[0])
			g.order = g.order[1:]
		}
	}
	g.mu.Unlock()
	return a, nil
}

func (g *Geocoder) getJSON(ctx context.Context, u string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(dst)
}

var vnSuffix = regexp.MustCompile(`,\s*(Việt Nam|Vietnam)$`)

func (g *Geocoder) google(ctx context.Context, ll string) (string, error) {
	var r struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
		Results      []struct {
			FormattedAddress string `json:"formatted_address"`
		} `json:"results"`
	}
	u := "https://maps.googleapis.com/maps/api/geocode/json?" + url.Values{
		"latlng": {ll}, "language": {"vi"}, "region": {"vn"}, "key": {g.googleKey}}.Encode()
	if err := g.getJSON(ctx, u, &r); err != nil {
		return "", err
	}
	if r.Status != "OK" && r.Status != "ZERO_RESULTS" {
		return "", fmt.Errorf("Google Geocoding: %s %s", r.Status, r.ErrorMessage)
	}
	if len(r.Results) == 0 {
		return "", nil
	}
	return vnSuffix.ReplaceAllString(r.Results[0].FormattedAddress, ""), nil
}

// Google: đang dùng Google Geocoding (có GOOGLE_MAPS_API_KEY).
func (g *Geocoder) Google() bool { return g.googleKey != "" }

// googleSearch: tìm địa chỉ bằng Google Geocoding, chỉ trong Việt Nam, tối đa 6 kết quả.
func (g *Geocoder) googleSearch(ctx context.Context, q string) ([]Place, error) {
	var r struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
		Results      []struct {
			FormattedAddress string   `json:"formatted_address"`
			Types            []string `json:"types"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}
	u := "https://maps.googleapis.com/maps/api/geocode/json?" + url.Values{
		"address": {q}, "components": {"country:VN"}, "language": {"vi"}, "region": {"vn"}, "key": {g.googleKey}}.Encode()
	if err := g.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	if r.Status != "OK" && r.Status != "ZERO_RESULTS" {
		return nil, fmt.Errorf("Google Geocoding: %s %s", r.Status, r.ErrorMessage)
	}
	out := []Place{}
	for _, x := range r.Results {
		wide := false // Google không hiểu địa chỉ thì trả tâm cả nước / cả tỉnh: bỏ
		for _, t := range x.Types {
			wide = wide || t == "country" || t == "administrative_area_level_1"
		}
		if !wide {
			out = append(out, Place{Name: vnSuffix.ReplaceAllString(x.FormattedAddress, ""), Lat: x.Geometry.Location.Lat, Lng: x.Geometry.Location.Lng})
		}
	}
	return mergePlaces(out, nil), nil
}

func (g *Geocoder) osm(ctx context.Context, ll string) (string, error) {
	// Nominatim cho tối đa 1 lượt/giây: xếp hàng toàn máy chủ
	g.nomiMu.Lock()
	defer g.nomiMu.Unlock()
	if wait := time.Until(g.nomiLast.Add(time.Second)); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	defer func() { g.nomiLast = time.Now() }()

	parts := strings.SplitN(ll, ",", 2)
	var r struct {
		Address map[string]string `json:"address"`
	}
	u := "https://nominatim.openstreetmap.org/reverse?" + url.Values{
		"format": {"jsonv2"}, "accept-language": {"vi"}, "zoom": {"18"}, "lat": {parts[0]}, "lon": {parts[1]}}.Encode()
	if err := g.getJSON(ctx, u, &r); err != nil {
		return "", err
	}
	a := r.Address
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(strings.TrimSpace(a["house_number"] + " " + a["road"]))
	for _, k := range []string{"hamlet", "neighbourhood", "quarter", "suburb", "village", "town", "city_district", "county", "city", "state"} {
		add(a[k])
	}
	return strings.Join(out, ", "), nil
}

// Place: 1 kết quả tìm địa điểm.
type Place struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Search: tìm địa điểm (trong Việt Nam), tối đa 6 kết quả, lưu đệm. Nguồn: Nominatim (xếp hàng 1 lượt/giây) và Photon
// (tìm tên địa điểm / công ty, gõ gần đúng tốt hơn). Không ra thì rút gọn câu tìm rồi thử lại (xem Variants):
// bỏ tên công ty ở đầu, bỏ "Quận 7 / Phường 5" (đơn vị cũ trước sáp nhập 2025), bỏ số nhà.
func (g *Geocoder) Search(ctx context.Context, q string) ([]Place, error) {
	q = strings.Join(strings.Fields(q), " ")
	if len([]rune(q)) < 3 {
		return []Place{}, nil
	}
	key := "q:" + strings.ToLower(q)
	g.mu.Lock()
	if v, ok := g.search[key]; ok {
		g.mu.Unlock()
		return v, nil
	}
	g.mu.Unlock()

	var out []Place
	var lastErr error
	for _, v := range Variants(q) {
		if g.googleKey != "" {
			// Có key: chỉ dùng Google (OSM lệch vài km ở VN); Google lỗi (hết hạn mức...) thì mới thử OSM
			ps, err := g.googleSearch(ctx, v)
			if err == nil {
				if len(ps) > 0 {
					out = ps
					break
				}
				continue
			}
			lastErr = err
		}
		ps, err := g.nominatim(ctx, v)
		if err != nil {
			lastErr = err
		}
		if ph, err := g.photon(ctx, v); err == nil {
			ps = mergePlaces(ps, ph)
		}
		if len(ps) > 0 {
			out = ps
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if out == nil {
		if lastErr != nil {
			return nil, lastErr
		}
		out = []Place{}
	}
	g.mu.Lock()
	if g.search == nil || len(g.search) > cacheMax {
		g.search = map[string][]Place{}
	}
	g.search[key] = out
	g.mu.Unlock()
	return out, nil
}

var (
	// đầu câu là tên công ty / cửa hàng (phần trước dấu phẩy đầu tiên)
	companyRE = regexp.MustCompile(`(?i)^(tổng công ty|công ty|cong ty|cty|tnhh|cổ phần|văn phòng|vp|chi nhánh|cửa hàng|showroom)(\s|$)`)
	// đơn vị cũ có số (trước sáp nhập 2025): Quận 7, Q.3, Phường 5, P.12
	oldUnitRE = regexp.MustCompile(`(?i)(^|[\s,])(quận|q\.|q|phường|p\.|p)\s*\d+`)
	// số nhà đầu câu: 45, 12/3, 25A, số 7
	houseNumRE = regexp.MustCompile(`(?i)^(số\s*)?[0-9][0-9a-z/\-]*\s+`)
)

// Variants: câu tìm gốc rồi các bản rút gọn dần (tối đa 4, không trùng).
func Variants(q string) []string {
	out := []string{}
	add := func(s string) {
		s = strings.Trim(strings.Join(strings.Fields(s), " "), " ,")
		if len([]rune(s)) < 3 || len(out) >= 4 {
			return
		}
		for _, o := range out {
			if strings.EqualFold(o, s) {
				return
			}
		}
		out = append(out, s)
	}
	add(q)
	parts := strings.Split(q, ",")
	if len(parts) > 1 && companyRE.MatchString(strings.TrimSpace(parts[0])) {
		q = strings.Join(parts[1:], ",")
		add(q)
	}
	q = oldUnitRE.ReplaceAllString(q, "")
	add(q)
	add(houseNumRE.ReplaceAllString(strings.TrimSpace(q), ""))
	return out
}

func (g *Geocoder) nominatim(ctx context.Context, q string) ([]Place, error) {
	g.nomiMu.Lock()
	defer g.nomiMu.Unlock()
	if wait := time.Until(g.nomiLast.Add(time.Second)); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() { g.nomiLast = time.Now() }()
	var r []struct {
		DisplayName string `json:"display_name"`
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
	}
	u := "https://nominatim.openstreetmap.org/search?" + url.Values{
		"format": {"jsonv2"}, "accept-language": {"vi"}, "countrycodes": {"vn"}, "limit": {"5"}, "q": {q}}.Encode()
	if err := g.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	out := []Place{}
	for _, x := range r {
		var p Place
		if _, err := fmt.Sscan(x.Lat, &p.Lat); err != nil {
			continue
		}
		if _, err := fmt.Sscan(x.Lon, &p.Lng); err != nil {
			continue
		}
		p.Name = vnSuffix.ReplaceAllString(x.DisplayName, "")
		out = append(out, p)
	}
	return out, nil
}

// photon: tìm bằng Photon (komoot), chỉ trong khung Việt Nam.
func (g *Geocoder) photon(ctx context.Context, q string) ([]Place, error) {
	var r struct {
		Features []struct {
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	u := "https://photon.komoot.io/api/?" + url.Values{"q": {q}, "limit": {"5"}, "bbox": {"102.1,8.3,109.6,23.4"}}.Encode()
	if err := g.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	out := []Place{}
	for _, f := range r.Features {
		pr := f.Properties
		str := func(k string) string { s, _ := pr[k].(string); return s }
		if c := str("countrycode"); c != "" && !strings.EqualFold(c, "VN") {
			continue
		}
		if len(f.Geometry.Coordinates) != 2 {
			continue
		}
		parts := []string{}
		seen := map[string]bool{}
		for _, v := range []string{str("name"), strings.TrimSpace(str("housenumber") + " " + str("street")), str("district"), str("city"), str("state")} {
			if v = strings.TrimSpace(v); v != "" && !seen[v] {
				seen[v] = true
				parts = append(parts, v)
			}
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, Place{Name: strings.Join(parts, ", "), Lat: f.Geometry.Coordinates[1], Lng: f.Geometry.Coordinates[0]})
	}
	return out, nil
}

// mergePlaces: gộp 2 danh sách, bỏ điểm trùng (cách nhau dưới ~20 m), tối đa 6.
func mergePlaces(a, b []Place) []Place {
	out := append([]Place{}, a...)
	for _, p := range b {
		dup := false
		for _, o := range out {
			if math.Abs(o.Lat-p.Lat) < 0.0002 && math.Abs(o.Lng-p.Lng) < 0.0002 {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}
