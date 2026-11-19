package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// Nộp ảnh không tạo ảnh thu nhỏ; lần đọc đầu tạo từ ảnh gốc rồi lưu lại.
func TestLazySmall(t *testing.T) {
	f, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 3000, 2000))
	for y := 0; y < 2000; y += 10 {
		for x := 0; x < 3000; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	h := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(h[:])
	if err := f.SavePhoto(sha, buf.Bytes(), nil); err != nil {
		t.Fatal(err)
	}
	small := filepath.Join(f.photos, sha+"_s.jpg")
	if _, err := os.Stat(small); !os.IsNotExist(err) {
		t.Fatal("chưa xem mà đã có ảnh thu nhỏ")
	}
	b, err := f.ReadPhoto(sha, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || max(cfg.Width, cfg.Height) != SmallSize {
		t.Fatalf("ảnh thu nhỏ %dx%d: %v", cfg.Width, cfg.Height, err)
	}
	if _, err := os.Stat(small); err != nil {
		t.Fatal("ảnh thu nhỏ chưa được lưu lại")
	}
}
