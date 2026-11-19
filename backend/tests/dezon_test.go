package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/service"
)

const dezonPage = `<div data-dc-plist>
<article data-pcard class="dznt-pcard"> <a href="/nha-thau/a/nha-pho-1/"><img data-src="/a.jpg"> <span>Hôm nay</span> </a><div><h3><a href="/nha-thau/a/nha-pho-1/">Nhà phố đường số 51, P Bình Trưng, TP HCM</a></h3><div> <span>Nhà thầu: <a href="/nha-thau/a/">Nhà thầu A</a></span></div></div></article>
<article data-pcard class="dznt-pcard"> <a href="/nha-thau/b/villa/"><img data-src="/b.jpg"> <span>27 ngày trước</span> </a><div><h3><a href="/nha-thau/b/villa/">Villa Indochine Tiền Giang</a></h3><div> <span>Nhà thầu: <a href="/nha-thau/b/">Nhà thầu B</a></span></div></div></article>
</div>`

type dzGeo struct {
	google bool
	calls  atomic.Int64
}

func (g *dzGeo) Google() bool { return g.google }
func (g *dzGeo) Search(_ context.Context, q string) ([]geo.Place, error) {
	g.calls.Add(1)
	if strings.Contains(q, "Bình Trưng") {
		return []geo.Place{{Name: q, Lat: 10.7919, Lng: 106.7752}}, nil
	}
	return nil, nil
}

func TestDezonSync(t *testing.T) {
	ctx := context.Background()
	page := dezonPage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(page)) }))
	defer srv.Close()
	repo := repository.NewDezonRepository(newDB(t))
	g := &dzGeo{}
	s := service.NewDezonSync(repo, g, srv.URL+"/nha-thau/du-an-moi/")

	added, located, err := s.Run(ctx)
	if err != nil || added != 2 || located != 1 {
		t.Fatalf("lần 1: added=%d located=%d err=%v", added, located, err)
	}
	pts, err := repo.Points(ctx, "")
	if err != nil || len(pts) != 1 {
		t.Fatalf("points: %v %+v", err, pts)
	}
	p := pts[0]
	if p.Title != "Nhà phố đường số 51, P Bình Trưng, TP HCM" || p.Contractor != "Nhà thầu A" || p.Province != "VN79" ||
		p.GeoSrc != "osm" || p.GeoLevel != "dia_chi" || p.Cover != srv.URL+"/a.jpg" || p.ContractorURL != srv.URL+"/nha-thau/a/" || p.PostedAt == nil {
		t.Fatalf("điểm: %+v", p)
	}
	if pts, _ := repo.Points(ctx, "VN01"); len(pts) != 0 {
		t.Fatalf("lọc tỉnh: %+v", pts)
	}

	// Lần 2: không có gì mới, không tra lại (công trình không tra được để 7 ngày sau mới thử lại)
	calls := g.calls.Load()
	if added, located, err := s.Run(ctx); err != nil || added != 0 || located != 0 || g.calls.Load() != calls {
		t.Fatalf("lần 2: added=%d located=%d err=%v calls=%d->%d", added, located, err, calls, g.calls.Load())
	}

	// Có key Google: tra lại các công trình đã tra bằng OSM
	g.google = true
	if _, located, err := s.Run(ctx); err != nil || located != 1 {
		t.Fatalf("google: located=%d err=%v", located, err)
	}
	if pts, _ := repo.Points(ctx, ""); len(pts) != 1 || pts[0].GeoSrc != "google" {
		t.Fatalf("google: %+v", pts)
	}

	// Nhà thầu đổi tiêu đề (địa chỉ): tra lại toạ độ; dezon.vn lỗi: vẫn giữ dữ liệu cũ
	page = strings.Replace(dezonPage, "đường số 51", "đường số 52", 2)
	if _, located, err := s.Run(ctx); err != nil || located != 1 {
		t.Fatalf("đổi tiêu đề: located=%d err=%v", located, err)
	}
	page = "<html>đổi giao diện</html>"
	if _, _, err := s.Run(ctx); err == nil {
		t.Fatal("trang không có thẻ nào phải báo lỗi")
	}
	if pts, _ := repo.Points(ctx, ""); len(pts) != 1 || !strings.Contains(pts[0].Title, "đường số 52") {
		t.Fatalf("sau lỗi: %+v", pts)
	}
}
