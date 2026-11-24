package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"

	"golang.org/x/image/draw"
)

// RelBox: vùng trên ảnh theo tỉ lệ (0..1), dùng chung cho mọi cỡ ảnh cùng tỉ lệ (ảnh thu nhỏ, ảnh thẻ).
type RelBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Rect: vùng điểm ảnh tương ứng trên ảnh cỡ b.
func (r RelBox) Rect(b image.Rectangle) image.Rectangle {
	w, h := float64(b.Dx()), float64(b.Dy())
	return image.Rect(b.Min.X+int(r.X*w), b.Min.Y+int(r.Y*h), b.Min.X+int((r.X+r.W)*w+0.999), b.Min.Y+int((r.Y+r.H)*h+0.999)).Intersect(b)
}

// Mask: che các vùng (SĐT) bằng ô vuông to (pixel hoá) rồi phủ mờ màu trung bình: không đọc lại được số,
// nhưng vẫn nhìn ra là có bảng hiệu. Trả ảnh mới, không sửa ảnh gốc.
func Mask(src image.Image, boxes []RelBox) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	for _, bx := range boxes {
		r := bx.Rect(dst.Bounds())
		if r.Empty() {
			continue
		}
		cell := max(6, r.Dy()/2) // ô to hơn nửa chiều cao chữ: không đọc được số
		for y := r.Min.Y; y < r.Max.Y; y += cell {
			for x := r.Min.X; x < r.Max.X; x += cell {
				c := image.Rect(x, y, min(x+cell, r.Max.X), min(y+cell, r.Max.Y))
				draw.Draw(dst, c, &image.Uniform{C: avg(dst, c)}, image.Point{}, draw.Src)
			}
		}
		// phủ thêm 1 lớp màu trung bình cả vùng (55%) cho chắc
		a := avg(dst, r)
		a.A = 140
		draw.Draw(dst, r, &image.Uniform{C: a}, image.Point{}, draw.Over)
	}
	return dst
}

func avg(img *image.RGBA, r image.Rectangle) color.RGBA {
	var sr, sg, sb, n uint64
	step := max(1, r.Dx()/8)
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			sr, sg, sb, n = sr+uint64(c.R), sg+uint64(c.G), sb+uint64(c.B), n+1
		}
	}
	if n == 0 {
		return color.RGBA{A: 255}
	}
	return color.RGBA{R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n), A: 255}
}

// BlurJPEG: làm mờ cả ảnh (không đọc được chữ nào): thu nhỏ còn cạnh dài 24 điểm ảnh rồi phóng lại.
// Dùng khi ảnh chưa quét chữ mà người xem đang bật "Ẩn thông tin".
func BlurJPEG(data []byte) ([]byte, error) {
	src, _, err := Decode(data)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	sw, sh := 24, max(1, 24*b.Dy()/max(1, b.Dx()))
	if b.Dy() > b.Dx() {
		sw, sh = max(1, 24*b.Dx()/max(1, b.Dy())), 24
	}
	small := image.NewRGBA(image.Rect(0, 0, sw, sh))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), src, b, draw.Src, nil)
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.BiLinear.Scale(dst, dst.Bounds(), small, small.Bounds(), draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// MaskJPEG: đọc JPEG, che vùng, nén lại JPEG chất lượng 85.
func MaskJPEG(data []byte, boxes []RelBox) ([]byte, error) {
	img, _, err := Decode(data)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, Mask(img, boxes), &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
