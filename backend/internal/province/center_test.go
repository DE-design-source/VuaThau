package province

import "testing"

func TestCenterInside(t *testing.T) {
	for _, p := range All() {
		lat, lng, ok := Center(p.Code)
		if !ok {
			t.Fatalf("%s: không có tâm", p.Name)
		}
		if at := At(lat, lng); at == nil || at.Code != p.Code {
			t.Errorf("%s: tâm %.4f,%.4f nằm ngoài tỉnh", p.Name, lat, lng)
		}
	}
}
