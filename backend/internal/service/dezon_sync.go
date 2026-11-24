package service

import (
	"context"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"vuathau/backend/internal/integration/dezon"
	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/pkg/safe"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// DezonGeocoder: tìm toạ độ theo địa chỉ (geo.Geocoder).
type DezonGeocoder interface {
	Search(ctx context.Context, q string) ([]geo.Place, error)
	Google() bool
}

// DezonSync: đọc trang dự án mới trên dezon.vn mỗi `every`, lưu công trình mới, tra toạ độ từ địa chỉ trong tiêu đề.
// Có key Google: tra lại bằng Google các công trình trước đó tra bằng OpenStreetMap (OSM lệch vài km).
type DezonSync struct {
	repo    repository.DezonRepository
	geo     DezonGeocoder
	pageURL string
	http    *http.Client
	now     func() time.Time
	mu      sync.Mutex // 1 lượt đồng bộ mỗi lúc
}

func NewDezonSync(repo repository.DezonRepository, g DezonGeocoder, pageURL string) *DezonSync {
	return &DezonSync{repo: repo, geo: g, pageURL: pageURL, http: &http.Client{Timeout: 30 * time.Second}, now: time.Now}
}

// Start: chạy ngay rồi lặp lại mỗi every.
func (s *DezonSync) Start(every time.Duration) {
	go func() {
		for {
			safe.Run("đồng bộ dự án dezon.vn", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
				defer cancel()
				if added, located, err := s.Run(ctx); err != nil {
					log.Printf("dezon dự án mới: %v", err)
				} else if added+located > 0 {
					log.Printf("dezon dự án mới: thêm %d, tra toạ độ %d", added, located)
				}
			})
			time.Sleep(every)
		}
	}()
}

// Run: 1 lượt đồng bộ. Trả số công trình mới và số công trình vừa tra được toạ độ.
// dezon.vn lỗi: vẫn tra toạ độ các công trình đã lưu.
func (s *DezonSync) Run(ctx context.Context) (added, located int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, ferr := dezon.FetchProjects(ctx, s.http, s.pageURL, s.now().In(VN))
	for _, p := range ps {
		isNew, err := s.repo.Upsert(ctx, repository.DezonUpsert{URL: p.URL, Title: p.Title, Contractor: p.Contractor,
			ContractorURL: p.ContractorURL, Cover: p.Cover, PostedAt: p.PostedAt})
		if err != nil {
			return added, located, err
		}
		if isNew {
			added++
		}
	}
	if s.geo == nil {
		return added, 0, ferr
	}
	google := s.geo.Google()
	todo, err := s.repo.NeedGeo(ctx, google, 7*24*time.Hour, 500)
	if err != nil {
		return added, 0, err
	}
	src := "osm"
	if google {
		src = "google"
	}
	for _, p := range todo {
		if ctx.Err() != nil {
			break
		}
		g := s.locate(ctx, p.Title)
		if g.Lat == nil && ctx.Err() != nil {
			break // hết giờ giữa chừng: không ghi "không tra được"
		}
		g.Src = src
		if err := s.repo.SetGeo(ctx, p.ID, g); err != nil {
			return added, located, err
		}
		if g.Lat != nil {
			located++
		}
	}
	return added, located, ferr
}

// wardCheckM: điểm theo số nhà / đường cách điểm phường, xã ghi trong địa chỉ quá xa: coi là tra nhầm, dùng điểm phường.
const wardCheckM = 4000

// locate: thử từng câu tìm (địa chỉ cụ thể trước, phường / xã sau); chỉ nhận điểm nằm đúng tỉnh ghi trong tiêu đề.
// Có cả điểm địa chỉ và điểm phường / xã mà 2 điểm cách nhau quá wardCheckM: dùng điểm phường (OSM hay nhầm đường cùng tên).
func (s *DezonSync) locate(ctx context.Context, title string) repository.DezonGeo {
	want := province.FromAddress(normAddr(title))
	find := func(q DezonQuery) *repository.DezonGeo {
		places, err := s.geo.Search(ctx, q.Text)
		if err != nil {
			return nil
		}
		for _, pl := range places {
			at := province.At(pl.Lat, pl.Lng)
			if at == nil || (want != nil && at.Code != want.Code) {
				continue
			}
			lat, lng := pl.Lat, pl.Lng
			return &repository.DezonGeo{Lat: &lat, Lng: &lng, Province: at.Code, Level: q.Level, Query: q.Text}
		}
		return nil
	}
	qs := DezonQueries(title)
	for i, q := range qs {
		g := find(q)
		if g == nil {
			continue
		}
		if q.Level == "dia_chi" {
			for _, w := range qs[i+1:] {
				if !areaRE.MatchString(w.Text) {
					continue
				}
				if wg := find(w); wg != nil {
					if distM(*g.Lat, *g.Lng, *wg.Lat, *wg.Lng) > wardCheckM {
						return *wg
					}
					break
				}
			}
		}
		return *g
	}
	g := repository.DezonGeo{}
	if want != nil {
		g.Province = want.Code
	}
	return g
}

// distM: khoảng cách 2 toạ độ (mét).
func distM(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371000.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := p2-p1, (lng2-lng1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// DezonQuery: 1 câu tìm toạ độ. Level: dia_chi (số nhà / đường / toà nhà) hoặc khu_vuc (phường / xã).
type DezonQuery struct{ Text, Level string }

var (
	// loại công trình đầu tiêu đề: "Nhà phố đường số 8, P Linh Xuân" -> "đường số 8, P Linh Xuân"
	kindRE = regexp.MustCompile(`(?i)^(nhà phố|nhà thép|villa|biệt thự( phố| tân cổ điển)?|office|văn phòng|căn hộ( dịch vụ| dich vụ)?|showroom|lâu đài|sales gallery|coffee|dự án|công trình( thực tế)?)(?:[\s,]+|$)`)
	// thửa đất / tờ bản đồ: không tìm được trên bản đồ
	parcelRE = regexp.MustCompile(`(?i)(thửa( đất)?( số)?\s*\d+|tờ (bản đồ|bđ)( số)?\s*:?\s*\d+|tbđ\s*:?\s*(số\s*)?\d+)`)
	timesRE  = regexp.MustCompile(`(?i)[,\s]*lần\s*\d+\s*$`)
	streetRE = regexp.MustCompile(`(?i)(\d|đường|hẻm|ngõ|kdc|khu đô thị|ttđt|tòa|toà|lô )`)
	areaRE   = regexp.MustCompile(`(?i)^(phường|xã|thị trấn|khu phố)\s`)
	oneRE    = regexp.MustCompile(`(?i)(đường|hẻm|ngõ|phường|xã|số \d)`)
	abbr     = []struct {
		re  *regexp.Regexp
		rep string
	}{
		{regexp.MustCompile(`(^|\s)dường\s`), "${1}đường "}, // gõ sai thường gặp
		{regexp.MustCompile(`(?i)(^|[\s,])(tp\.?\s*hcm|tphcm|hcm)\b`), "${1}Hồ Chí Minh"},
		{regexp.MustCompile(`(?i)(^|[\s,])tp\.?\s+`), "${1}Thành phố "},
		{regexp.MustCompile(`(^|[\s,])[Pp]\.?\s+`), "${1}Phường "},
		{regexp.MustCompile(`(^|[\s,])[Xx]\.?\s+`), "${1}Xã "},
		{regexp.MustCompile(`(^|[\s,])(Kp|KP|kp)\.?\s+`), "${1}Khu phố "},
		{regexp.MustCompile(`(^|[\s,])[Qq]\.\s*`), "${1}Quận "},
	}
)

// normAddr: viết đủ các chữ tắt (P -> Phường, TP HCM -> Hồ Chí Minh...).
func normAddr(s string) string {
	for _, a := range abbr {
		s = a.re.ReplaceAllString(s, a.rep)
	}
	return strings.Join(strings.Fields(s), " ")
}

// DezonQueries: các câu tìm toạ độ từ tiêu đề công trình (tiêu đề nhiều dòng đã nối bằng ", ").
func DezonQueries(title string) []DezonQuery {
	out := []DezonQuery{}
	add := func(text, level string) {
		text = strings.Trim(strings.Join(strings.Fields(text), " "), " ,-")
		// 1 cụm không dấu phẩy chỉ nhận khi có đường / phường / xã (bỏ "Villa Indochine Tiền Giang", "Villa Quận 7")
		if len([]rune(text)) < 6 || (!strings.Contains(text, ",") && !oneRE.MatchString(text)) {
			return
		}
		for _, o := range out {
			if strings.EqualFold(o.Text, text) {
				return
			}
		}
		out = append(out, DezonQuery{text, level})
	}
	t := timesRE.ReplaceAllString(normAddr(title), "")
	// Tách các cụm theo dấu phẩy, bỏ loại công trình ở đầu, bỏ cụm thửa đất / tờ bản đồ
	var parts []string
	for _, p := range strings.Split(t, ",") {
		p = strings.TrimSpace(kindRE.ReplaceAllString(strings.TrimSpace(p), ""))
		if p = strings.TrimSpace(parcelRE.ReplaceAllString(p, "")); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return out
	}
	last := len(parts) - 1
	// Địa chỉ: từ cụm đầu tiên không phải phường / xã (và không phải cụm tỉnh cuối cùng, trừ khi chỉ có 1 cụm)
	for i, p := range parts {
		if areaRE.MatchString(p) || (i == last && last > 0) {
			continue
		}
		level := "khu_vuc"
		if streetRE.MatchString(p) {
			level = "dia_chi"
		}
		add(strings.Join(parts[i:], ", "), level)
		break
	}
	// Phường / xã + tỉnh
	for i, p := range parts {
		if areaRE.MatchString(p) && i < last {
			add(p+", "+parts[last], "khu_vuc")
		}
	}
	if len(parts) == 1 && areaRE.MatchString(parts[0]) {
		add(parts[0], "khu_vuc")
	}
	// Cuối cùng: 2 cụm cuối (vd "Ân Thi, Hưng Yên", "Đông Hòa, Hồ Chí Minh")
	if last >= 1 && !areaRE.MatchString(parts[last-1]) {
		add(parts[last-1]+", "+parts[last], "khu_vuc")
	}
	return out
}
