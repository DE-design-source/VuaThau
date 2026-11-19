package service

import (
	"context"
	"reflect"
	"testing"

	"vuathau/backend/internal/integration/geo"
)

func TestDezonQueries(t *testing.T) {
	cases := map[string][]DezonQuery{
		"Nhà phố đường số 51, P Bình Trưng, TP HCM lần 2": {
			{"đường số 51, Phường Bình Trưng, Hồ Chí Minh", "dia_chi"}, {"Phường Bình Trưng, Hồ Chí Minh", "khu_vuc"}},
		"Thửa Đất Số 2735, Tờ Bản Đồ 31, Phường Bình Dương, 92 An Mỹ - Phú Mỹ, Bình Dương, Hồ Chí Minh": {
			{"92 An Mỹ - Phú Mỹ, Bình Dương, Hồ Chí Minh", "dia_chi"}, {"Phường Bình Dương, Hồ Chí Minh", "khu_vuc"},
			{"Bình Dương, Hồ Chí Minh", "khu_vuc"}},
		"Nhà phố dường Lê Hữu Kiều, P Bình Trưng, TP HCM": {
			{"đường Lê Hữu Kiều, Phường Bình Trưng, Hồ Chí Minh", "dia_chi"}, {"Phường Bình Trưng, Hồ Chí Minh", "khu_vuc"}},
		"Lâu đài đường Đông Hội X. Đông Anh Tp Hà Nội": {{"đường Đông Hội Xã Đông Anh Thành phố Hà Nội", "dia_chi"}},
		"Biệt thự Indochine, Ân Thi, Hưng yên":         {{"Indochine, Ân Thi, Hưng yên", "khu_vuc"}, {"Ân Thi, Hưng yên", "khu_vuc"}},
		"Villa Indochine Tiền Giang":                   {},
		"Villa Quận 7":                                 {},
		"Công trình thực tế":                           {},
	}
	for title, want := range cases {
		if got := DezonQueries(title); !reflect.DeepEqual(got, want) {
			t.Errorf("%q:\n got  %q\n want %q", title, got, want)
		}
	}
}

type fakeGeo map[string][]geo.Place

func (f fakeGeo) Search(_ context.Context, q string) ([]geo.Place, error) { return f[q], nil }
func (f fakeGeo) Google() bool                                            { return false }

func TestDezonLocate(t *testing.T) {
	const title = "Nhà phố đường Tân Thắng, P Tân Phú, TP HCM"
	addr, ward := "đường Tân Thắng, Phường Tân Phú, Hồ Chí Minh", "Phường Tân Phú, Hồ Chí Minh"
	inHCM := geo.Place{Lat: 10.7925, Lng: 106.6286}
	wardPt := geo.Place{Lat: 10.7870, Lng: 106.6330}
	hanoi := geo.Place{Lat: 21.0285, Lng: 105.8542}
	farHCM := geo.Place{Lat: 11.2936, Lng: 106.3822} // cùng tỉnh nhưng cách phường ~60 km

	// Điểm ngoài tỉnh ghi trong địa chỉ: bỏ, lấy kết quả sau
	s := &DezonSync{geo: fakeGeo{addr: {hanoi, inHCM}, ward: {wardPt}}}
	if g := s.locate(context.Background(), title); g.Lat == nil || *g.Lat != inHCM.Lat || g.Level != "dia_chi" || g.Province != "VN79" {
		t.Fatalf("got %+v", g)
	}
	// Điểm địa chỉ cách điểm phường quá xa: dùng điểm phường
	s = &DezonSync{geo: fakeGeo{addr: {farHCM}, ward: {wardPt}}}
	if g := s.locate(context.Background(), title); g.Lat == nil || *g.Lat != wardPt.Lat || g.Level != "khu_vuc" {
		t.Fatalf("far: got %+v", g)
	}
	// Không tra được: không có toạ độ, vẫn có tỉnh theo địa chỉ
	s = &DezonSync{geo: fakeGeo{}}
	if g := s.locate(context.Background(), title); g.Lat != nil || g.Province != "VN79" {
		t.Fatalf("none: got %+v", g)
	}
}
