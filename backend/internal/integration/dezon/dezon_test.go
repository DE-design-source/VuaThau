package dezon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 1 thẻ thật của dezon.vn (18/11/2026, bỏ bớt style) + 1 thẻ trùng link + 1 thẻ dự án (không phải nhà thầu).
const sample = `<div><article data-card="" data-name="kiến phong" data-tags="nhà phố|biệt thự|căn hộ" class="dcx-1" style="border:1px solid #E8E8E8"><div><a class="dznt-cover" href="https://dezon.vn/nha-thau/kien-phong/" aria-label="Kiến Phong"></a><noscript><img class="dz-dc-img" src="/wp-content/uploads/dz-data/nt/kien-phong/Apo.jpg" alt="x"></noscript><img src='data:image/svg+xml,%3Csvg%3E' class="lazyload dz-dc-img" data-src="/wp-content/uploads/dz-data/nt/kien-phong/Apo.jpg" alt="x"> <span data-verified="" style="x">Đề xuất</span></div><div><div><h3 style="font-size:17px"><a class="dznt-name" href="https://dezon.vn/nha-thau/kien-phong/">Kiến Phong</a></h3><p style="margin:0">TP. HCM · <strong>7</strong> công trình · <strong>8</strong> năm</p></div><div> <span>Nhà phố</span></div> <a href="https://dezon.vn/nha-thau/kien-phong/" class="dcx-2">Xem hồ sơ →</a></div></article>
<article data-card="" data-name="kiến phong" class="dcx-1"><h3><a class="dznt-name" href="https://dezon.vn/nha-thau/kien-phong/">Kiến Phong</a></h3></article>
<article data-card="" data-name="b &amp; c" data-tags="" class="dcx-1"><h3><a class="dznt-name" href="/nha-thau/b-c/">B &amp; C</a></h3><p>Đà Nẵng · <strong>12</strong> công trình</p></article>
<article data-pcard class="dznt-pcard"><a href="https://dezon.vn/nha-thau/tan-tai/nha-pho/">Nhà phố</a></article></div>`

func TestParse(t *testing.T) {
	rows := Parse(sample, ListURL)
	if len(rows) != 2 {
		t.Fatalf("rows: %+v", rows)
	}
	a := rows[0]
	if a.Name != "Kiến Phong" || a.URL != "https://dezon.vn/nha-thau/kien-phong/" || a.Province != "TP. HCM" || a.Projects != 7 ||
		a.Years != 8 || !a.Featured || a.Cover != "https://dezon.vn/wp-content/uploads/dz-data/nt/kien-phong/Apo.jpg" ||
		len(a.Tags) != 3 || a.Tags[0] != "Nhà phố" {
		t.Fatalf("a: %+v", a)
	}
	b := rows[1]
	if b.Name != "B & C" || b.URL != "https://dezon.vn/nha-thau/b-c/" || b.Projects != 12 || b.Years != 0 || b.Featured || b.Cover != "" {
		t.Fatalf("b: %+v", b)
	}
}

func TestCacheAndStale(t *testing.T) {
	calls, fail := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			http.Error(w, "x", 500)
			return
		}
		_, _ = w.Write([]byte(sample))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Hour)
	ctx := context.Background()
	l, err := c.Get(ctx)
	if err != nil || len(l.Rows) != 2 {
		t.Fatal(err, l)
	}
	if _, _ = c.Get(ctx); calls != 1 {
		t.Fatalf("phải dùng bản đệm: %d lần gọi", calls)
	}
	c.last.At = time.Now().Add(-2 * time.Hour) // hết hạn, dezon.vn lỗi: trả bản cũ
	fail = true
	l, err = c.Get(ctx)
	if err != nil || !l.Stale || len(l.Rows) != 2 {
		t.Fatalf("stale: %v %+v", err, l)
	}
	c2 := New(srv.URL, time.Hour) // chưa có bản nào: báo lỗi
	if _, err := c2.Get(ctx); err == nil {
		t.Fatal("phải lỗi")
	}
}
