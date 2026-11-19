package service

import (
	"testing"
)

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

