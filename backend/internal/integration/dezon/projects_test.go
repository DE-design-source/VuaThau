package dezon

import (
	"testing"
	"time"
)

const pcards = `<div data-dc-plist><article data-pcard style="border: 1px solid #E8E8E8" class="dznt-pcard"> <a href="https://dezon.vn/nha-thau/dat-tin/thua-dat-so-2735/" style="position: relative"> <noscript><img src="/wp-content/uploads/dz-data/nt/dat-tin/a.jpg" alt="x"></noscript><img src='data:image/svg+xml,%3Csvg%3E' class="lazyload" data-src="/wp-content/uploads/dz-data/nt/dat-tin/a.jpg" alt="x"> <span style="position: absolute">Hôm nay</span> </a><div style="padding: 18px"><h3 style="font-size: 16px"><a href="https://dezon.vn/nha-thau/dat-tin/thua-dat-so-2735/" style="color: #1A1A1A">Thửa Đất Số 2735, Tờ Bản Đồ 31, Phường Bình Dương
92 An Mỹ - Phú Mỹ, Bình Dương, Hồ Chí Minh</a></h3><div style="display: flex"> <span style="color: #666">Nhà thầu: <a href="https://dezon.vn/nha-thau/dat-tin/" style="color: #1A1A1A">Đạt Tín</a></span> <a href="https://dezon.vn/nha-thau/dat-tin/thua-dat-so-2735/">Chi tiết ›</a></div></div></article>
<article data-pcard class="dznt-pcard"> <a href="/nha-thau/kien-phong/green-pool-villa/"><img data-src="/b.jpg"> <span>27 ngày trước</span> </a><div><h3><a href="/nha-thau/kien-phong/green-pool-villa/">Green Pool Villa Vĩnh Long &amp; co</a></h3><div> <span>Nhà thầu: <a href="/nha-thau/kien-phong/">Kiến Phong</a></span></div></div></article>
<article data-pcard class="dznt-pcard"> <a href="/nha-thau/kien-phong/green-pool-villa/"><span>Hôm qua</span> </a><h3><a href="/nha-thau/kien-phong/green-pool-villa/">Trùng link</a></h3></article></div>`

func TestParseProjects(t *testing.T) {
	now := time.Date(2026, 11, 19, 8, 30, 0, 0, time.UTC)
	ps := ParseProjects(pcards, ProjectsURL, now)
	if len(ps) != 2 {
		t.Fatalf("got %d projects: %+v", len(ps), ps)
	}
	a := ps[0]
	if a.URL != "https://dezon.vn/nha-thau/dat-tin/thua-dat-so-2735/" || a.Contractor != "Đạt Tín" || a.ContractorURL != "https://dezon.vn/nha-thau/dat-tin/" {
		t.Errorf("card 1: %+v", a)
	}
	if len(a.Lines) != 2 || a.Lines[1] != "92 An Mỹ - Phú Mỹ, Bình Dương, Hồ Chí Minh" {
		t.Errorf("lines: %q", a.Lines)
	}
	if a.Cover != "https://dezon.vn/wp-content/uploads/dz-data/nt/dat-tin/a.jpg" || a.Age != "Hôm nay" || !a.PostedAt.Equal(time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("cover/age: %q %q %v", a.Cover, a.Age, a.PostedAt)
	}
	b := ps[1]
	if b.URL != "https://dezon.vn/nha-thau/kien-phong/green-pool-villa/" || b.Title != "Green Pool Villa Vĩnh Long & co" || b.Contractor != "Kiến Phong" {
		t.Errorf("card 2: %+v", b)
	}
	if !b.PostedAt.Equal(time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("27 ngày trước: %v", b.PostedAt)
	}
}

func TestPostedAt(t *testing.T) {
	now := time.Date(2026, 11, 19, 23, 0, 0, 0, time.UTC)
	for age, want := range map[string]string{
		"Hôm nay": "2026-11-19", "Hôm qua": "2026-11-18", "2 ngày trước": "2026-11-17", "3 tuần trước": "2026-10-29",
		"2 tháng trước": "2026-09-19", "1 năm trước": "2025-11-19", "05/03/2026": "2026-03-05", "lạ": "2026-11-19",
	} {
		if got := PostedAt(age, now).Format("2006-01-02"); got != want {
			t.Errorf("%q: got %s want %s", age, got, want)
		}
	}
}
