package ocr

// Dải chữ ứng dụng chụp ảnh (Timemark...) in ở góc dưới bên trái ảnh: giờ chụp, địa chỉ, toạ độ GPS. Ảnh CTV gửi
// qua Messenger mất EXIF / GPS, nhưng dải chữ này còn: đọc nó để lấy vị trí ảnh và tách công trình (góp ý 25/11/2026).

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "image/jpeg" // giải mã JPEG
)

// StampReader: đọc chữ của dải chữ in góc dưới ảnh (Tesseract có; bản giả trong test có thể có).
type StampReader interface {
	Stamp(ctx context.Context, jpeg []byte) (string, error)
}

// StampImage: phần dưới bên trái ảnh (nơi in dải chữ), chữ trắng thành chữ đen trên nền trắng, phóng to cho dễ đọc.
// Trả PNG.
func StampImage(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 200 || h < 200 {
		return nil, fmt.Errorf("ảnh quá nhỏ")
	}
	x0, y0, x1, y1 := b.Min.X, b.Min.Y+h*62/100, b.Min.X+w*80/100, b.Max.Y
	// chữ dải cao ~2% cạnh dài; phóng để chữ cao ~40px (Tesseract đọc tốt nhất 30-50px)
	scale := 1
	if long := max(w, h); long < 2400 {
		scale = 2
	}
	out := image.NewGray(image.Rect(0, 0, (x1-x0)*scale, (y1-y0)*scale))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			r, g, bl = r>>8, g>>8, bl>>8
			lo, hi := min(r, g, bl), max(r, g, bl)
			v := uint8(255)
			if lo >= 200 && hi-lo <= 45 { // trắng (chữ in), không tính màu nhạt
				v = 0
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					out.SetGray((x-x0)*scale+dx, (y-y0)*scale+dy, color.Gray{Y: v})
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// stampLangs: có dữ liệu tiếng Việt thì đọc cả dấu (địa chỉ), không thì chỉ tiếng Anh (vẫn đọc được số, toạ độ).
func (t tesseract) stampLangs() string {
	out, err := exec.Command(t.bin, "--list-langs").CombinedOutput()
	if err == nil && strings.Contains(string(out), "\nvie") {
		return "vie+eng"
	}
	return "eng"
}

func (t tesseract) Stamp(ctx context.Context, src []byte) (string, error) {
	pic, err := StampImage(src)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	t.st.once.Do(func() { t.st.lang = t.stampLangs() })
	cmd := exec.CommandContext(ctx, t.bin, "stdin", "stdout", "-l", t.st.lang, "--psm", "6")
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	cmd.Stdin = bytes.NewReader(pic)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
