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
