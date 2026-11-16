// Package geo: địa chỉ của toạ độ GPS để in lên ảnh camera. Có key Google thì dùng Google Geocoding,
// không thì OpenStreetMap Nominatim (miễn phí, tối đa 1 lượt/giây, bắt buộc User-Agent nêu tên app).
package geo

import (
	"context"
	"encoding/json"
	"fmt"
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

	mu    sync.Mutex
	cache map[string]string // toạ độ làm tròn ~11m -> địa chỉ
	order []string

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
