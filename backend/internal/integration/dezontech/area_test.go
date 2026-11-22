package dezontech

import "testing"

func TestAreaFromNote(t *testing.T) {
	for _, c := range []struct{ note, kv, tinh string }{
		{"Địa chỉ: Hà Nội\nCó đất, xây mới nhà phố", "Hà Nội", ""},
		{"Thiết kế - Thi công nhà phố\nĐC: TP.HCM\nQuy mô 3 tầng", "TP.HCM", "TP. Hồ Chí Minh"},
		{"Dự án quán Cà phê Bắc Ninh - Tìm đơn vị thi công", "", "Bắc Ninh"},
		{"Đang có đất, chuẩn bị xây mới", "", ""},
	} {
		kv, tinh := areaFromNote(c.note)
		if kv != c.kv || tinh != c.tinh {
			t.Errorf("%q: %q %q, cần %q %q", c.note, kv, tinh, c.kv, c.tinh)
		}
	}
}
