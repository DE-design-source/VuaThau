package model

import (
	"sync"
	"testing"
)

// foldName gọi song song (nhiều request mở ảnh cùng lúc) không được panic, kết quả đúng.
func TestFoldNameConcurrent(t *testing.T) {
	in := []string{"Công ty TNHH Xây dựng Hùng Phát", "Nguyễn Văn Đức", "CTY CP KIẾN TRÚC ĐỒNG NAI 2025", "Trần Thị Ngọc Ánh"}
	want := make([]string, len(in))
	for i, s := range in {
		want[i] = foldName(s)
	}
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 300; k++ {
				i := (g + k) % len(in)
				if got := foldName(in[i]); got != want[i] {
					t.Errorf("foldName(%q) = %q, muốn %q", in[i], got, want[i])
					return
				}
			}
		}()
	}
	wg.Wait()
	if want[1] != "nguyenvanduc" {
		t.Fatalf("foldName: %q", want[1])
	}
}
