package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"golang.org/x/image/draw"
)

// ảnh cỡ app chụp: 1920x1440 JPEG
func benchJPEG(b *testing.B) []byte {
	b.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1440))
	for y := 0; y < 1440; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 7 % 251), uint8(y * 5 % 241), uint8((x + y) % 239), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func BenchmarkOpen(b *testing.B) {
	data := benchJPEG(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, _, err := Open(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDHash(b *testing.B) {
	img, _, _, _ := Open(benchJPEG(b))
	b.ReportAllocs()
	for b.Loop() {
		DHash(img)
	}
}

func BenchmarkSmallJPEG1568(b *testing.B) {
	img, _, _, _ := Open(benchJPEG(b))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := SmallJPEG(img, 1568); err != nil {
			b.Fatal(err)
		}
	}
}

func benchScale(b *testing.B, s draw.Scaler) {
	img, _, _, _ := Open(benchJPEG(b))
	src := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, 1568, 1176))
	b.ReportAllocs()
	for b.Loop() {
		s.Scale(dst, dst.Bounds(), img, src, draw.Src, nil)
	}
}

func BenchmarkScaleCatmullRom(b *testing.B)     { benchScale(b, draw.CatmullRom) }
func BenchmarkScaleBiLinear(b *testing.B)       { benchScale(b, draw.BiLinear) }
func BenchmarkScaleApproxBiLinear(b *testing.B) { benchScale(b, draw.ApproxBiLinear) }
