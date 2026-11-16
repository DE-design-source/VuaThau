// Package imaging: đọc ảnh tải lên (JPG/PNG), EXIF giờ chụp + GPS, xoay đúng chiều, thu nhỏ, băm dHash chống trùng.
package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // đăng ký bộ giải mã PNG
	"math"
	"math/bits"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // nhận thêm ảnh WebP
)

// Info: thông tin đọc từ EXIF. ExifTime dạng "YYYY-MM-DD HH:MM" (giờ ghi trên máy chụp), rỗng nếu không có.
type Info struct {
	ExifTime string
	Lat, Lng *float64
}

var ErrNotImage = errors.New("không đọc được file ảnh (chỉ nhận JPG/PNG)")

// Open giải mã ảnh, đọc EXIF (trước khi xoay) và trả ảnh đã xoay đúng chiều. Trả thêm định dạng: jpeg | png | webp.
func Open(data []byte) (image.Image, Info, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, Info{}, "", ErrNotImage
	}
	info, orient := readExif(data)
	return orient8(img, orient), info, format, nil
}

func readExif(data []byte) (Info, int) {
	var info Info
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return info, 1
	}
	// DateTimeOriginal, không có thì DateTime. EXIF không ghi múi giờ: là giờ máy chụp (VN).
	for _, f := range []exif.FieldName{exif.DateTimeOriginal, exif.DateTime} {
		if tag, err := x.Get(f); err == nil {
			if s, err := tag.StringVal(); err == nil {
				if t, err := time.Parse("2006:01:02 15:04:05", strings.TrimSpace(strings.TrimRight(s, "\x00"))); err == nil {
					info.ExifTime = t.Format("2006-01-02 15:04")
					break
				}
			}
		}
	}
	if lat, lng, err := x.LatLong(); err == nil && !math.IsNaN(lat) && !math.IsNaN(lng) && (lat != 0 || lng != 0) {
		info.Lat, info.Lng = &lat, &lng
	}
	orient := 1
	if tag, err := x.Get(exif.Orientation); err == nil {
		if v, err := tag.Int(0); err == nil {
			orient = v
		}
	}
	return info, orient
}

// orient8 xoay / lật ảnh theo thẻ EXIF Orientation (1..8).
func orient8(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := o >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case 2:
				nx, ny = w-1-x, y
			case 3:
				nx, ny = w-1-x, h-1-y
			case 4:
				nx, ny = x, h-1-y
			case 5:
				nx, ny = y, x
			case 6:
				nx, ny = h-1-y, x
			case 7:
				nx, ny = h-1-y, w-1-x
			case 8:
				nx, ny = y, w-1-x
			}
			dst.Set(nx, ny, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// Fit thu nhỏ ảnh để cạnh dài nhất <= max (không phóng to).
func Fit(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	k := float64(max) / math.Max(float64(w), float64(h))
	nw, nh := int(math.Round(float64(w)*k)), int(math.Round(float64(h)*k))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// SmallJPEG: ảnh thu nhỏ (cạnh dài <= max) dạng JPEG chất lượng 85, nền trắng cho ảnh trong suốt.
func SmallJPEG(src image.Image, max int) ([]byte, error) {
	img := Fit(src, max)
	b := img.Bounds()
	flat := image.NewRGBA(b)
	draw.Draw(flat, b, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, b, img, b.Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// DHash: băm cảm nhận 64 bit (ảnh xám 9x8, so sánh điểm kề nhau), dạng 16 ký tự hex.
// Ảnh đã nén lại / đổi cỡ vẫn cho dHash gần giống: khác <= 6 bit coi là cùng ảnh.
func DHash(src image.Image) string {
	g := image.NewGray(image.Rect(0, 0, 9, 8))
	s := Fit(src, 256) // thu nhỏ trước cho nhanh và mượt
	draw.ApproxBiLinear.Scale(g, g.Bounds(), s, s.Bounds(), draw.Src, nil)
	var v uint64
	for i := 0; i < 64; i++ {
		row, col := i/8, i%8
		if g.GrayAt(col, row).Y > g.GrayAt(col+1, row).Y {
			v |= 1 << uint(i)
		}
	}
	return fmt.Sprintf("%016x", v)
}

// Hamming: số bit khác nhau giữa 2 dHash.
func Hamming(a, b string) int {
	var x, y uint64
	if _, err := fmt.Sscanf(a, "%x", &x); err != nil {
		return 64
	}
	if _, err := fmt.Sscanf(b, "%x", &y); err != nil {
		return 64
	}
	return bits.OnesCount64(x ^ y)
}

// MimeOf: kiểu MIME theo định dạng giải mã.
func MimeOf(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// ExtOf: đuôi file theo kiểu MIME.
func ExtOf(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	default:
		return "jpg"
	}
}

// Meters: khoảng cách 2 toạ độ (Haversine).
func Meters(lat1, lng1, lat2, lng2 float64) float64 {
	p := math.Pi / 180
	h := math.Pow(math.Sin((lat2-lat1)*p/2), 2) + math.Cos(lat1*p)*math.Cos(lat2*p)*math.Pow(math.Sin((lng2-lng1)*p/2), 2)
	return 12_742_000 * math.Asin(math.Sqrt(h))
}
