package service

import (
	"context"
	"testing"

	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/repository"
)

// Dự án không ghim: chỉ nhận toạ độ tra được khi đúng tỉnh và tên có phường / đường đã nhập (ghim sai tệ hơn chưa ghim).
func TestNTLocate(t *testing.T) {
	const addr = "20 Nguyễn Thiện Thành, Phường Tân Khánh, TP. Hồ Chí Minh"
	chua := geo.Place{Name: "Thiền viện Nguyên Thủy, 33A Đường Số 10, Cát Lái, Phường Cát Lái, Hồ Chí Minh", Lat: 10.7803, Lng: 106.7642}
	hanoi := geo.Place{Name: "Nguyễn Thiện Thành, Ba Đình, Hà Nội", Lat: 21.0285, Lng: 105.8542}
	dung := geo.Place{Name: "Nguyễn Thiện Thành, Phường Tân Khánh, Hồ Chí Minh", Lat: 10.79, Lng: 106.72}
	cases := []struct {
		name   string
		places []geo.Place
		want   *geo.Place
	}{
		{"cùng tỉnh nhưng chỗ khác: bỏ", []geo.Place{chua}, nil},
		{"đúng tên đường nhưng khác tỉnh: bỏ", []geo.Place{hanoi}, nil},
		{"lấy điểm đúng đầu tiên", []geo.Place{chua, hanoi, dung}, &dung},
	}
	for _, c := range cases {
		s := (&NTService{}).WithGeo(fakeGeo{addr: c.places})
		p := &repository.NTProject{Address: addr, Province: "TP. Hồ Chí Minh", Ward: "Phường Tân Khánh", Street: "20 Nguyễn Thiện Thành"}
		s.locate(context.Background(), p)
		switch {
		case c.want == nil && p.Lat != nil:
			t.Errorf("%s: không được ghim, ra %v,%v", c.name, *p.Lat, *p.Lng)
		case c.want != nil && (p.Lat == nil || *p.Lat != c.want.Lat):
			t.Errorf("%s: muốn %v, ra %v", c.name, c.want.Lat, p.Lat)
		}
	}
}
