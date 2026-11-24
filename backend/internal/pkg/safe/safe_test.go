package safe

import (
	"sync"
	"testing"
)

func TestRunRecovers(t *testing.T) {
	if Run("thử", func() { var m map[string]int; m["x"] = 1 }) {
		t.Fatal("panic phải trả false")
	}
	if !Run("thử", func() {}) {
		t.Fatal("chạy xong phải trả true")
	}
}

func TestGoRecovers(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	Go("thử", func() {
		defer wg.Done()
		panic("lỗi")
	})
	wg.Wait() // tiến trình test còn sống là đạt
}
