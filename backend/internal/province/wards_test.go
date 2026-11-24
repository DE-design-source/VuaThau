package province

import "testing"

func TestWardAndSameProvince(t *testing.T) {
	if n := len(Divisions()); n != 34 {
		t.Fatalf("tỉnh: %d", n)
	}
	p, w, ok := Ward("thành phố hồ chí minh", "phuong an khanh")
	if !ok || p != "TP. Hồ Chí Minh" || w != "Phường An Khánh" {
		t.Fatalf("Ward: %q %q %v", p, w, ok)
	}
	if _, _, ok := Ward("TP. Hồ Chí Minh", "Phường Ba Đình"); ok {
		t.Fatal("phường Hà Nội không thuộc TP.HCM")
	}
	// tên tỉnh trong ranh giới (At) và trong danh sách phường phải khớp nhau, cả 34 tỉnh
	for _, d := range Divisions() {
		found := false
		for _, a := range All() {
			if SameProvince(a.Name, d.Name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tỉnh %q không khớp tên nào trong ranh giới", d.Name)
		}
	}
	if at := At(10.78, 106.70); at == nil || !SameProvince(at.Name, "TP. Hồ Chí Minh") {
		t.Fatalf("At HCM: %+v", at)
	}
}
