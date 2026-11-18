// Package dezon đọc danh sách "Nhà thầu tiềm năng" công khai trên dezon.vn (trang Sale xem kèm nhà thầu của hệ thống).
// Chỉ đọc trang HTML công khai; giữ bản đọc được gần nhất trong bộ nhớ, làm mới mỗi TTL.
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
	"sync"
	"time"
)

// ListURL: trang danh sách nhà thầu tiềm năng.
const ListURL = "https://dezon.vn/nha-thau/tiem-nang/"

// Contractor: 1 thẻ nhà thầu trên dezon.vn.
type Contractor struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`   // trang chi tiết trên dezon.vn
	Cover    string   `json:"cover"` // ảnh công trình (link tuyệt đối)
	Province string   `json:"province"`
	Projects int      `json:"projects"`
	Years    int      `json:"years"`
	Tags     []string `json:"tags"`
	Featured bool     `json:"featured"` // nhãn "Đề xuất"
}

// List: kết quả trả cho trang Sale.
type List struct {
	Rows   []Contractor `json:"rows"`
	At     time.Time    `json:"at"`     // lúc đọc dezon.vn
	Source string       `json:"source"` // link trang gốc
	Stale  bool         `json:"stale"`  // lần đọc mới nhất lỗi, đang dùng bản cũ
}

type Client struct {
	url  string
	ttl  time.Duration
	http *http.Client

	mu   sync.Mutex
	last *List
}

func New(listURL string, ttl time.Duration) *Client {
	return &Client{url: listURL, ttl: ttl, http: &http.Client{Timeout: 25 * time.Second}}
}

// Get: danh sách (dùng bản đệm nếu còn hạn; dezon.vn lỗi thì trả bản cũ có cờ stale).
func (c *Client) Get(ctx context.Context) (*List, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last != nil && time.Since(c.last.At) < c.ttl {
		return c.last, nil
	}
	rows, err := c.fetch(ctx)
	if err != nil {
		if c.last != nil {
			old := *c.last
			old.Stale = true
			return &old, nil
		}
		return nil, err
	}
	c.last = &List{Rows: rows, At: time.Now(), Source: c.url}
	return c.last, nil
}

func (c *Client) fetch(ctx context.Context) ([]Contractor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; VuaThau/1.0)")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("đọc dezon.vn: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dezon.vn trả mã %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	rows := Parse(string(b), c.url)
	if len(rows) == 0 {
		return nil, errors.New("không đọc được nhà thầu nào trên dezon.vn (trang đổi giao diện?)")
	}
	return rows, nil
}

var (
	nameRE  = regexp.MustCompile(`<a[^>]*class="dznt-name"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	nameRE2 = regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*class="dznt-name"[^>]*>([\s\S]*?)</a>`)
	coverRE = regexp.MustCompile(`data-src="([^"]+)"`)
	imgRE   = regexp.MustCompile(`<img[^>]*src="([^"]+)"`)
	pRE     = regexp.MustCompile(`<p[^>]*>([\s\S]*?)</p>`)
	tagsRE  = regexp.MustCompile(`data-tags="([^"]*)"`)
	tagRE   = regexp.MustCompile(`<[^>]+>`)
	numRE   = regexp.MustCompile(`(\d+)\s*(công trình|năm)`)
)

// Parse: các thẻ <article data-card> của trang danh sách (bỏ trùng theo link).
func Parse(page, base string) []Contractor {
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
	text := func(s string) string {
		return strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(s, " "))), " ")
	}
	out := []Contractor{}
	seen := map[string]bool{}
	parts := strings.Split(page, "<article data-card")
	for _, p := range parts[1:] {
		if i := strings.Index(p, "</article>"); i >= 0 {
			p = p[:i]
		}
		m := nameRE.FindStringSubmatch(p)
		if m == nil {
			m = nameRE2.FindStringSubmatch(p)
		}
		if m == nil {
			continue
		}
		c := Contractor{URL: abs(m[1]), Name: text(m[2]), Tags: []string{}}
		if c.Name == "" || seen[c.URL] {
			continue
		}
		seen[c.URL] = true
		if x := coverRE.FindStringSubmatch(p); x != nil {
			c.Cover = abs(x[1])
		} else if x := imgRE.FindStringSubmatch(p); x != nil && !strings.HasPrefix(x[1], "data:") {
			c.Cover = abs(x[1])
		}
		if x := pRE.FindStringSubmatch(p); x != nil {
			line := text(x[1])
			c.Province = strings.TrimSpace(strings.Split(line, "·")[0])
			if numRE.MatchString(c.Province) {
				c.Province = ""
			}
			for _, n := range numRE.FindAllStringSubmatch(line, -1) {
				v, _ := strconv.Atoi(n[1])
				if n[2] == "năm" {
					c.Years = v
				} else {
					c.Projects = v
				}
			}
		}
		if x := tagsRE.FindStringSubmatch(p); x != nil {
			for _, t := range strings.Split(html.UnescapeString(x[1]), "|") {
				if t = strings.TrimSpace(t); t != "" {
					r := []rune(t)
					c.Tags = append(c.Tags, strings.ToUpper(string(r[0]))+string(r[1:]))
				}
			}
		}
		c.Featured = strings.Contains(p, "data-verified")
		out = append(out, c)
	}
	return out
}
