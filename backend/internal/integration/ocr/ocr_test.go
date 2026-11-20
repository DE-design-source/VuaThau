package ocr

import (
	"strings"
	"testing"
)

// TSV Tesseract: level page block par line word left top width height conf text
const sample = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"5\t1\t1\t1\t1\t1\t100\t50\t200\t40\t95\tNHÀ THẦU\n" +
	"5\t1\t1\t1\t2\t1\t100\t100\t90\t40\t90\tHotline:\n" +
	"5\t1\t1\t1\t2\t2\t200\t100\t80\t40\t91\t0905\n" +
	"5\t1\t1\t1\t2\t3\t290\t100\t60\t40\t92\t123\n" +
	"5\t1\t1\t1\t2\t4\t360\t100\t60\t40\t93\t456\n" +
	// toạ độ Timemark, giờ, ngày: không phải SĐT
	"5\t1\t2\t1\t1\t1\t50\t900\t300\t30\t90\t10.825746°N,\n" +
	"5\t1\t2\t1\t1\t2\t360\t900\t300\t30\t90\t106.717299°E\n" +
	"5\t1\t3\t1\t1\t1\t50\t800\t100\t30\t90\t09:45\n" +
	"5\t1\t3\t1\t2\t1\t50\t850\t200\t30\t90\t2026.11.15\n" +
	// SĐT liền 1 chữ, có dấu chấm; số 0 đọc nhầm thành chữ O
	"5\t1\t4\t1\t1\t1\t500\t300\t250\t40\t80\tO236.3888.999\n" +
	// tổng đài
	"5\t1\t5\t1\t1\t1\t500\t400\t100\t40\t80\t1900\n" +
	"5\t1\t5\t1\t1\t2\t610\t400\t100\t40\t80\t6868\n"

func TestPhoneBoxes(t *testing.T) {
	words, err := ParseTSV(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	boxes := PhoneBoxes(words, 1000, 1000, nil)
	if len(boxes) != 3 {
		t.Fatalf("tìm được %d SĐT, cần 3: %+v", len(boxes), boxes)
	}
	// SĐT đầu: 0905 123 456 trải từ x=200 đến 420, có nới rộng
	b := boxes[0]
	if b.X > 0.2 || b.X+b.W < 0.42 || b.Y > 0.1 || b.Y+b.H < 0.14 {
		t.Fatalf("vùng SĐT 1 chưa phủ hết: %+v", b)
	}
	if b.X < 0.17 { // không lan sang chữ "Hotline:" (kết thúc ở x=190) quá nhiều
		t.Fatalf("vùng SĐT 1 lan quá rộng: %+v", b)
	}
}

// OCR chỉ đọc được "300 900" của bảng hiệu 0889 300 900 (ảnh thật HS 28): che theo SĐT nhà thầu đã biết, nới sang trái
func TestKnownPartial(t *testing.T) {
	words := []Word{{Text: "300", Line: "30-1-1", X: 697, Y: 621, W: 15, H: 28, Conf: 53}, {Text: "900", Line: "30-1-1", X: 713, Y: 621, W: 22, H: 28, Conf: 43}}
	if b := PhoneBoxes(words, 1176, 1568, nil); len(b) != 0 {
		t.Fatalf("không biết SĐT mà vẫn che: %+v", b)
	}
	b := PhoneBoxes(words, 1176, 1568, []string{"0889 300 900"})
	if len(b) != 1 {
		t.Fatalf("che theo SĐT đã biết: %+v", b)
	}
	if x0 := b[0].X * 1176; x0 > 697-20 {
		t.Fatalf("chưa nới sang phần 0889 bị sót: x0=%.0f", x0)
	}
	if b := PhoneBoxes([]Word{{Text: "2026", Line: "1", X: 1, Y: 1, W: 9, H: 9}}, 100, 100, []string{"0912026111"}); len(b) != 0 {
		t.Fatalf("chuỗi ngắn trùng 1 phần SĐT: %+v", b)
	}
}

// Ảnh thật (HS 243, bảng hiệu IVY ARCHI): OCR đọc dính biểu tượng điện thoại vào số: "©093586" "1417,"
func TestIconGlued(t *testing.T) {
	words := []Word{
		{Text: "hotine:", Line: "2-1-1", X: 558, Y: 953, W: 34, H: 10, Conf: 83},
		{Text: "©093586", Line: "3-1-1", X: 492, Y: 965, W: 92, H: 32, Conf: 6},
		{Text: "1417,", Line: "3-1-1", X: 595, Y: 972, W: 54, H: 17, Conf: 49},
		{Text: "®", Line: "4-1-1", X: 492, Y: 997, W: 17, H: 19, Conf: 83},
		{Text: "0943", Line: "4-1-1", X: 517, Y: 999, W: 43, H: 17, Conf: 94},
		{Text: "545", Line: "4-1-1", X: 568, Y: 1000, W: 31, H: 17, Conf: 94},
		{Text: "444", Line: "4-1-1", X: 605, Y: 1001, W: 31, H: 15, Conf: 96},
		{Text: "Hotline:0905123456", Line: "5-1-1", X: 100, Y: 1100, W: 200, H: 20, Conf: 90},
	}
	if b := PhoneBoxes(words, 1176, 1568, nil); len(b) != 3 {
		t.Fatalf("cần che 3 SĐT, được %d: %+v", len(b), b)
	}
	for _, s := range []string{"hotline", "IVY", "2026", "©"} {
		if numPart(s) != s && digitsOf(s) == "" {
			t.Errorf("numPart(%q) = %q", s, numPart(s))
		}
	}
}

func TestIsPhone(t *testing.T) {
	for d, want := range map[string]bool{
		"0905123456": true, "02363888999": true, "84905123456": true, "19006868": true, "18001234": true,
		"0012345678": false, "10825746": false, "202611": false, "0945": false, "106717299": false,
	} {
		if isPhone(d) != want {
			t.Errorf("isPhone(%s) = %v", d, !want)
		}
	}
}

func TestMerge(t *testing.T) {
	m := Merge([]Box{{X: 0.1, Y: 0.1, W: 0.2, H: 0.05}, {X: 0.11, Y: 0.1, W: 0.18, H: 0.05}, {X: 0.5, Y: 0.5, W: 0.1, H: 0.1}})
	if len(m) != 2 {
		t.Fatalf("gộp: %+v", m)
	}
}
