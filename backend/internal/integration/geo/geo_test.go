package geo

import (
	"reflect"
	"testing"
)

func TestVariants(t *testing.T) {
	cases := map[string][]string{
		// địa chỉ công ty: bỏ tên công ty, bỏ quận cũ, bỏ số nhà
		"Công ty TNHH ABC, 45 Nguyễn Văn Linh, Quận 7": {
			"Công ty TNHH ABC, 45 Nguyễn Văn Linh, Quận 7",
			"45 Nguyễn Văn Linh, Quận 7",
			"45 Nguyễn Văn Linh",
			"Nguyễn Văn Linh",
		},
		"Cty Xây dựng Hàm Rồng, 12/3 Lê Lợi, P.5, Q.3": {
			"Cty Xây dựng Hàm Rồng, 12/3 Lê Lợi, P.5, Q.3",
			"12/3 Lê Lợi, P.5, Q.3",
			"12/3 Lê Lợi",
			"Lê Lợi",
		},
		// tên địa điểm / công ty không kèm địa chỉ: giữ nguyên
		"Vinamilk":    {"Vinamilk"},
		"Landmark 81": {"Landmark 81"},
		// "Ấp 3" không phải "P 3": giữ nguyên
		"Ấp 3, xã Tân Thạnh": {"Ấp 3, xã Tân Thạnh"},
		// địa chỉ mới (sau sáp nhập): chỉ bỏ số nhà
		"395 Bùi Thị Xuân, Phường Bình Dương": {"395 Bùi Thị Xuân, Phường Bình Dương", "Bùi Thị Xuân, Phường Bình Dương"},
	}
	for q, want := range cases {
		if got := Variants(q); !reflect.DeepEqual(got, want) {
			t.Errorf("Variants(%q)\n got %q\nwant %q", q, got, want)
		}
	}
}

func TestMergePlaces(t *testing.T) {
	a := []Place{{Name: "A", Lat: 10.7, Lng: 106.7}}
	b := []Place{{Name: "A gần trùng", Lat: 10.70005, Lng: 106.70005}, {Name: "B", Lat: 10.8, Lng: 106.8}}
	got := mergePlaces(a, b)
	if len(got) != 2 || got[1].Name != "B" {
		t.Fatalf("merge: %+v", got)
	}
}
