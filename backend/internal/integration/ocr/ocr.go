// Package ocr: tìm vị trí số điện thoại trên ảnh công trình (bảng hiệu nhà thầu, băng rôn...) bằng Tesseract OCR
// chạy trên máy chủ, để làm mờ SĐT khi sale / CTV / Google Drive xem ảnh. Admin, kế toán vẫn xem ảnh gốc.
package ocr

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/pkg/imaging"
)

// Box: vùng cần làm mờ, toạ độ theo tỉ lệ ảnh (0..1) để dùng được cho mọi cỡ ảnh cùng tỉ lệ.
type Box = imaging.RelBox

// Finder: tìm các vùng có SĐT trên ảnh JPEG.
type Finder interface {
	Enabled() bool
	// known: SĐT nhà thầu đã biết của hồ sơ (bắt thêm số OCR chỉ đọc được 1 phần).
	Phones(ctx context.Context, jpeg []byte, known []string) ([]Box, error)
}

type disabled struct{}

func (disabled) Enabled() bool                                           { return false }
func (disabled) Phones(context.Context, []byte, []string) ([]Box, error) { return nil, nil }

// Disabled: không có Tesseract (máy dev Windows): không làm mờ.
func Disabled() Finder { return disabled{} }

// slots: số lượt Tesseract chạy cùng lúc. Mỗi lượt (1 luồng, ảnh 1568px) tốn ~330MB RAM, ~2-4 giây.
var slots = make(chan struct{}, 2)

type tesseract struct{ bin string }

// New: dùng lệnh tesseract nếu có trong PATH, không có thì Disabled().
func New() Finder {
	bin, err := exec.LookPath("tesseract")
	if err != nil {
		return Disabled()
	}
	return tesseract{bin: bin}
}

func (tesseract) Enabled() bool { return true }

// Phones: 2 lượt đọc (psm 11: chữ rời rạc như bảng hiệu; psm 3: đoạn chữ), gộp vùng tìm được.
func (t tesseract) Phones(ctx context.Context, img []byte, known []string) ([]Box, error) {
	cfg, err := imageSize(img)
	if err != nil {
		return nil, err
	}
	var out []Box
	for _, psm := range []string{"11", "3"} {
		words, err := t.run(ctx, img, psm)
		if err != nil {
			return nil, err
		}
		out = append(out, PhoneBoxes(words, cfg[0], cfg[1], known)...)
	}
	return Merge(out), nil
}

func (t tesseract) run(ctx context.Context, img []byte, psm string) ([]Word, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	cmd := exec.CommandContext(ctx, t.bin, "stdin", "stdout", "-l", "eng", "--psm", psm, "tsv")
	// mặc định Tesseract chạy nhiều luồng: ~600MB RAM / ảnh; 1 luồng ~330MB, chậm hơn không đáng kể
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	cmd.Stdin = bytes.NewReader(img)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseTSV(&stdout)
}

// Word: 1 chữ Tesseract đọc được. Line: khoá dòng (block-par-line).
type Word struct {
	Text       string
	Line       string
	X, Y, W, H int
	Conf       float64
}

// ParseTSV: đọc kết quả "tsv" của Tesseract (level 5 = từng chữ).
func ParseTSV(r io.Reader) ([]Word, error) {
	cr := csv.NewReader(r)
	cr.Comma = '\t'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	out := []Word{}
	for i, f := range rows {
		if i == 0 || len(f) < 12 || f[0] != "5" {
			continue
		}
		n := func(k int) int { v, _ := strconv.Atoi(f[k]); return v }
		conf, _ := strconv.ParseFloat(f[10], 64)
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		out = append(out, Word{Text: text, Line: f[2] + "-" + f[3] + "-" + f[4], X: n(6), Y: n(7), W: n(8), H: n(9), Conf: conf})
	}
	return out, nil
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// phoneish: chữ có thể là 1 phần SĐT: chỉ gồm số và . - ( ) + / : (OCR hay đọc "0905.123.456", "(0236) 3 888 999").
// Chữ "O"/"o" giữa các số thường là số 0 đọc nhầm.
func phoneish(s string) bool {
	if digitsOf(s) == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', strings.ContainsRune(".-()+/:,", r), r == 'O', r == 'o':
		default:
			return false
		}
	}
	return true
}

// numPart: phần từ con số đầu tiên của chữ: OCR hay đọc dính biểu tượng điện thoại / nhãn vào số ("©093586",
// "Hotline:0905", "ĐT:0236..."). Giữ "+" / "(" / chữ O (số 0 đọc nhầm) đứng ngay trước số.
func numPart(s string) string {
	i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if i <= 0 {
		return s
	}
	if c := s[i-1]; c == '+' || c == '(' || c == 'O' || c == 'o' {
		i--
	}
	return s[i:]
}

// isPhone: chuỗi số giống SĐT Việt Nam: 0xxxxxxxxx (10-11 số), 84xxxxxxxxx, hoặc tổng đài 1800 / 1900 (8-10 số).
func isPhone(d string) bool {
	switch {
	case strings.HasPrefix(d, "84") && len(d) >= 11 && len(d) <= 12:
		return true
	case strings.HasPrefix(d, "0") && len(d) >= 10 && len(d) <= 11 && d[1] != '0':
		return true
	case (strings.HasPrefix(d, "1800") || strings.HasPrefix(d, "1900")) && len(d) >= 8 && len(d) <= 10:
		return true
	}
	return false
}

// PhoneBoxes: ghép các chữ liền nhau trên cùng dòng thành chuỗi số, chuỗi nào giống SĐT thì lấy vùng bao (nới rộng
// một chút cho chắc che hết). w, h: cỡ ảnh để quy về tỉ lệ 0..1. known: SĐT đã biết; chuỗi >= 6 số nằm trong 1 SĐT
// đã biết (OCR đọc sót vài số, vd chỉ ra "300 900" của 0889 300 900) cũng che, và nới vùng che sang phần số bị sót.
func PhoneBoxes(words []Word, w, h int, known []string) []Box {
	if w <= 0 || h <= 0 {
		return nil
	}
	kn := []string{}
	for _, k := range known {
		d := digitsOf(k)
		if strings.HasPrefix(d, "84") && len(d) >= 11 {
			d = "0" + d[2:]
		}
		if len(d) >= 9 {
			kn = append(kn, d)
		}
	}
	out := []Box{}
	flush := func(run []Word) {
		if len(run) == 0 {
			return
		}
		all := ""
		for _, x := range run {
			all += digitsOf(strings.NewReplacer("O", "0", "o", "0").Replace(numPart(x.Text)))
		}
		// phần số bị sót (theo SĐT đã biết): trước / sau chuỗi đọc được
		before, after := 0, 0
		// "Hotline: 0905 123 456 - 0236 3 888 999": 2 số liền nhau cũng che cả cụm
		if !isPhone(all) && !(len(all) >= 18 && len(all) <= 23 && (isPhone(all[:10]) || isPhone(all[:11]))) {
			hit := false
			if len(all) >= 6 {
				for _, k := range kn {
					if i := strings.Index(k, all); i >= 0 {
						before, after, hit = i, len(k)-i-len(all), true
						break
					}
				}
			}
			if !hit {
				return
			}
		}
		x0, y0, x1, y1 := run[0].X, run[0].Y, run[0].X+run[0].W, run[0].Y+run[0].H
		for _, x := range run[1:] {
			x0, y0 = min(x0, x.X), min(y0, x.Y)
			x1, y1 = max(x1, x.X+x.W), max(y1, x.Y+x.H)
		}
		if per := float64(x1-x0) / float64(len(all)); before+after > 0 {
			// mỗi số rộng ~per, thêm khoảng trắng giữa các cụm số
			x0 -= int(per * float64(before) * 1.4)
			x1 += int(per * float64(after) * 1.4)
		}
		pad := float64(y1-y0) * 0.35
		bx := Box{X: (float64(x0) - pad) / float64(w), Y: (float64(y0) - pad) / float64(h),
			W: (float64(x1-x0) + 2*pad) / float64(w), H: (float64(y1-y0) + 2*pad) / float64(h)}
		out = append(out, clamp(bx))
	}
	var run []Word
	for _, x := range words {
		ok := phoneish(numPart(x.Text)) && x.Conf >= 0 // conf -1: dòng trống
		near := len(run) > 0 && run[len(run)-1].Line == x.Line &&
			x.X-(run[len(run)-1].X+run[len(run)-1].W) <= 3*max(x.H, run[len(run)-1].H)
		if ok && (len(run) == 0 || near) {
			run = append(run, x)
			continue
		}
		flush(run)
		run = nil
		if ok {
			run = []Word{x}
		}
	}
	flush(run)
	return out
}

func clamp(b Box) Box {
	if b.X < 0 {
		b.W += b.X
		b.X = 0
	}
	if b.Y < 0 {
		b.H += b.Y
		b.Y = 0
	}
	b.W = min(b.W, 1-b.X)
	b.H = min(b.H, 1-b.Y)
	return b
}

// Merge: bỏ vùng trùng (2 lượt đọc cùng tìm ra 1 SĐT): vùng nằm gần trọn trong vùng khác thì gộp.
func Merge(in []Box) []Box {
	out := []Box{}
	for _, b := range in {
		merged := false
		for i, o := range out {
			if overlap(b, o) > 0.5 {
				x0, y0 := min(b.X, o.X), min(b.Y, o.Y)
				x1, y1 := max(b.X+b.W, o.X+o.W), max(b.Y+b.H, o.Y+o.H)
				out[i] = Box{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, b)
		}
	}
	return out
}

// overlap: diện tích giao / diện tích vùng nhỏ hơn.
func overlap(a, b Box) float64 {
	w := min(a.X+a.W, b.X+b.W) - max(a.X, b.X)
	h := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	small := min(a.W*a.H, b.W*b.H)
	if small <= 0 {
		return 0
	}
	return w * h / small
}
