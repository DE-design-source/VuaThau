// Package safe: chạy việc nền (goroutine) mà không để 1 panic làm tắt cả máy chủ.
// net/http chỉ tự bắt panic trong handler; panic trong goroutine khác (đồng bộ Lark, Messenger, AI, OCR...)
// làm thoát cả tiến trình, mọi lượt tải ảnh đang chạy mất theo.
package safe

import (
	"log"
	"runtime/debug"
)

// Run chạy fn; panic thì ghi nhật ký kèm stack và trả false (fn chạy xong bình thường: true).
func Run(name string, fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC %s: %v\n%s", name, r, debug.Stack())
			ok = false
		}
	}()
	fn()
	return true
}

// Go chạy fn trong goroutine mới, có bắt panic như Run.
func Go(name string, fn func()) {
	go Run(name, fn)
}
