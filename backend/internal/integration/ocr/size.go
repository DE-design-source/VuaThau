package ocr

import (
	"bytes"
	"image"
	_ "image/jpeg" // đăng ký bộ giải mã
	_ "image/png"
)

// imageSize: [rộng, cao] của ảnh (chỉ đọc phần đầu file).
func imageSize(b []byte) ([2]int, error) {
	c, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{c.Width, c.Height}, nil
}
