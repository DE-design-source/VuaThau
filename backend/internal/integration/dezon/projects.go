package dezon

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ProjectsURL: trang "Dự án mới cập nhật" — công trình nhà thầu vừa đăng, xếp theo ngày đăng.
const ProjectsURL = "https://dezon.vn/nha-thau/du-an-moi/"

// Project: 1 thẻ công trình (article data-pcard). Trang chi tiết trên dezon.vn hay trả 404, nên chỉ dùng dữ liệu của thẻ.
type Project struct {
	URL           string
	Title         string   // các dòng tiêu đề nối bằng ", "
	Lines         []string // tiêu đề theo dòng (nhiều thẻ ghi "thửa đất..." 1 dòng, số nhà + đường dòng sau)
	Contractor    string
	ContractorURL string
	Cover         string
	Age           string    // nhãn trên ảnh: "Hôm nay", "Hôm qua", "27 ngày trước"...
	PostedAt      time.Time // ngày đăng suy từ Age (theo lúc đọc trang)
}

var (
	pTitleRE = regexp.MustCompile(`<h3[^>]*>\s*<a[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	pCtorRE  = regexp.MustCompile(`Nhà thầu:\s*<a[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	pAgeRE   = regexp.MustCompile(`<span[^>]*>([^<]{2,40})</span>\s*</a>`)
	ageNumRE = regexp.MustCompile(`(?i)^(\d+)\s*(ngày|tuần|tháng|năm)\s*trước$`)
	ageDayRE = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})$`)
)

// FetchProjects đọc trang dự án mới. Trang không có thẻ nào: lỗi (dezon.vn đổi giao diện?).
func FetchProjects(ctx context.Context, hc *http.Client, pageURL string, now time.Time) ([]Project, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; VuaThau/1.0)")
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("đọc dezon.vn: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dezon.vn trả mã %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	ps := ParseProjects(string(b), pageURL, now)
	if len(ps) == 0 {
		return nil, errors.New("không đọc được công trình nào trên dezon.vn (trang đổi giao diện?)")
	}
	return ps, nil
}

// ParseProjects: các thẻ <article data-pcard> (bỏ trùng theo link).
func ParseProjects(page, base string, now time.Time) []Project {
	bu, _ := url.Parse(base)
	abs := func(s string) string {
		s = html.UnescapeString(s)
		if bu == nil {
			return s
		}
		if u, err := bu.Parse(s); err == nil {
			return u.String()
		}
		return s
	}
	out := []Project{}
	seen := map[string]bool{}
	parts := strings.Split(page, "<article data-pcard")
	for _, p := range parts[1:] {
		if i := strings.Index(p, "</article>"); i >= 0 {
			p = p[:i]
		}
		m := pTitleRE.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		pr := Project{URL: abs(m[1])}
		raw := html.UnescapeString(tagRE.ReplaceAllString(strings.ReplaceAll(m[2], "<br", "\n<br"), " "))
		for _, l := range strings.Split(raw, "\n") {
			if l = strings.Join(strings.Fields(l), " "); l != "" {
				pr.Lines = append(pr.Lines, l)
			}
		}
		pr.Title = strings.Join(pr.Lines, ", ")
		if pr.Title == "" || seen[pr.URL] {
			continue
		}
		seen[pr.URL] = true
		if x := pCtorRE.FindStringSubmatch(p); x != nil {
			pr.ContractorURL = abs(x[1])
			pr.Contractor = strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(x[2], " "))), " ")
		}
		if x := coverRE.FindStringSubmatch(p); x != nil {
			pr.Cover = abs(x[1])
		} else if x := imgRE.FindStringSubmatch(p); x != nil && !strings.HasPrefix(x[1], "data:") {
			pr.Cover = abs(x[1])
		}
		if x := pAgeRE.FindStringSubmatch(p); x != nil {
			pr.Age = strings.TrimSpace(html.UnescapeString(x[1]))
		}
		pr.PostedAt = PostedAt(pr.Age, now)
		out = append(out, pr)
	}
	return out
}

// PostedAt: ngày đăng từ nhãn "Hôm nay / Hôm qua / N ngày (tuần, tháng, năm) trước / dd/mm/yyyy". Không hiểu: hôm nay.
func PostedAt(age string, now time.Time) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	a := strings.ToLower(strings.TrimSpace(age))
	switch {
	case a == "hôm qua":
		return day.AddDate(0, 0, -1)
	case ageNumRE.MatchString(a):
		m := ageNumRE.FindStringSubmatch(a)
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "tuần":
			return day.AddDate(0, 0, -7*n)
		case "tháng":
			return day.AddDate(0, -n, 0)
		case "năm":
			return day.AddDate(-n, 0, 0)
		}
		return day.AddDate(0, 0, -n)
	case ageDayRE.MatchString(a):
		m := ageDayRE.FindStringSubmatch(a)
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, now.Location())
	}
	return day
}
