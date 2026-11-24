package ocr

import "testing"

func TestTextBoxes(t *testing.T) {
	words := []Word{
		{Text: "XÂY", Line: "1-1-1", X: 100, Y: 100, W: 60, H: 30, Conf: 90},
		{Text: "DỰNG", Line: "1-1-1", X: 170, Y: 100, W: 80, H: 30, Conf: 85},
		{Text: "~", Line: "2-1-1", X: 10, Y: 500, W: 5, H: 5, Conf: 95},      // 1 ký tự: bỏ
		{Text: "abc", Line: "3-1-1", X: 300, Y: 300, W: 40, H: 20, Conf: 20}, // tin cậy thấp: bỏ
	}
	b := TextBoxes(words, 1000, 1000)
	if len(b) != 1 || b[0].X > 0.1 || b[0].X+b[0].W < 0.25 {
		t.Fatalf("TextBoxes: %+v", b)
	}
}
