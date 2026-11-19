package service

import (
	"testing"

	"vuathau/backend/internal/integration/inbox"
)

func TestPickImages(t *testing.T) {
	im := func(k string, extra bool) inbox.Image { return inbox.Image{Key: k, Extra: extra} }
	all := []inbox.Image{im("le1", true), im("le2", true), im("le3", true), im("own1", false), im("le4", true), im("own2", false)}
	got := pickImages(all, 4)
	keys := ""
	for _, g := range got {
		keys += g.Key + " "
	}
	// giữ 2 ảnh của hồ sơ + 2 ảnh lẻ mới nhất (le4, le3), theo thứ tự gửi
	if keys != "le3 own1 le4 own2 " {
		t.Fatalf("pick: %q", keys)
	}
	if len(pickImages(all[:3], 4)) != 3 {
		t.Fatal("ít hơn max: giữ hết")
	}
}

func TestMoreDetailed(t *testing.T) {
	for _, c := range []struct {
		addr, old string
		want      bool
	}{
		{"07 Hối Kiếng 08, P. Ngũ Hành Sơn, Đà Nẵng", "Đà Nẵng", true},
		{"Đà Nẵng333", "Hòa Xuân - Đà Nẵng", false}, // số dính chữ, ngắn hơn
		{"Đà Nẵng333", "Đà Nẵng", false},
		{"12/3 Lê Lợi, Huế", "Huế", true},
		{"70 Ngô Nhân Tịnh", "65 Lê Duẩn, Đà Nẵng", false}, // địa chỉ cũ đã có số
		{"", "Đà Nẵng", false},
	} {
		if got := moreDetailed(c.addr, c.old); got != c.want {
			t.Errorf("moreDetailed(%q, %q) = %v", c.addr, c.old, got)
		}
	}
}
