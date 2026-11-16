package province

import "testing"

func TestAll34(t *testing.T) {
	ps := All()
	if len(ps) != 34 {
		t.Fatalf("có %d tỉnh, muốn 34", len(ps))
	}
	merged := 0
	for _, p := range ps {
		if len(p.Merged) > 0 {
			merged++
		}
	}
	if merged != 23 {
		t.Fatalf("%d tỉnh mới do sáp nhập, muốn 23 (11 tỉnh giữ nguyên)", merged)
	}
}

func TestAt(t *testing.T) {
	for _, c := range []struct {
		lat, lng float64
		want     string
	}{
		{21.0285, 105.8542, "Hà Nội"},              // Hồ Gươm
		{10.7769, 106.7009, "TP. Hồ Chí Minh"},     // Quận 1 cũ
		{11.0705, 106.6829, "TP. Hồ Chí Minh"},     // Bình Dương cũ (hồ sơ Messenger)
		{10.3460, 107.0843, "TP. Hồ Chí Minh"},     // Vũng Tàu cũ
		{18.6601, 105.6851, "Nghệ An"},             // Vinh
		{16.0544, 108.2022, "Đà Nẵng"},             // Đà Nẵng
		{15.8801, 108.3380, "Đà Nẵng"},             // Hội An (Quảng Nam cũ)
		{12.2388, 109.1967, "Khánh Hòa"},           // Nha Trang
		{11.9404, 108.4583, "Lâm Đồng"},            // Đà Lạt
		{10.9333, 108.1000, "Lâm Đồng"},            // Phan Thiết (Bình Thuận cũ)
		{10.0452, 105.7469, "Cần Thơ"},             // Ninh Kiều
		{20.4388, 106.1621, "Ninh Bình"},           // Nam Định cũ
		{13.7830, 109.2196, "Gia Lai"},             // Quy Nhơn (Bình Định cũ)
		{10.0124, 105.0809, "An Giang"},            // Rạch Giá (Kiên Giang cũ)
		{16.4637, 107.5909, "Huế"},                 // Huế
	} {
		p := At(c.lat, c.lng)
		if p == nil || p.Name != c.want {
			t.Errorf("At(%v, %v) = %v, muốn %s", c.lat, c.lng, p, c.want)
		}
	}
	if p := At(13.0, 113.0); p != nil {
		t.Errorf("giữa biển phải nil, được %s", p.Name)
	}
}

func TestFromAddress(t *testing.T) {
	for addr, want := range map[string]string{
		"395 Đ. Bùi Thị Xuân, Bình Dương, Hồ Chí Minh": "TP. Hồ Chí Minh",
		"Thủ Dầu Một, tỉnh Bình Dương":                 "TP. Hồ Chí Minh",
		"P. Trường Vinh, Nghệ An":                      "Nghệ An",
		"12 Lê Lợi, Quận 1, TP HCM":                    "TP. Hồ Chí Minh",
		"Phường 7, TP. Vũng Tàu, Bà Rịa - Vũng Tàu":    "TP. Hồ Chí Minh",
		"Xã Tân Phú, huyện Đồng Phú, Bình Phước":       "Đồng Nai",
		"Hội An, Quảng Nam":                            "Đà Nẵng",
		"Thành phố Huế, Thừa Thiên Huế":                "Huế",
		"Gia Nghĩa, Đắk Nông":                          "Lâm Đồng",
		"Cầu Giấy, Hà Nội":                             "Hà Nội",
		"Bạc Liêu":                                     "Cà Mau",
	} {
		p := FromAddress(addr)
		if p == nil || p.Name != want {
			t.Errorf("FromAddress(%q) = %v, muốn %s", addr, p, want)
		}
	}
	if p := FromAddress("không rõ"); p != nil {
		t.Errorf("phải nil, được %s", p.Name)
	}
}
