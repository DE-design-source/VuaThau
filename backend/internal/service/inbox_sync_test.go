package service

import (
	"math"
	"testing"

	"vuathau/backend/internal/model"
)

func TestParseCoord(t *testing.T) {
	for in, want := range map[string][2]float64{
		"11.070510°N, 106.682880°E": {11.070510, 106.682880},
		"18.660076°N, 105.685129°E": {18.660076, 105.685129},
		"10.7769, 106.7009":         {10.7769, 106.7009},
		"33.8688°S 151.2093°E":      {-33.8688, 151.2093},
	} {
		lat, lng := parseCoord(in)
		if lat == nil || math.Abs(*lat-want[0]) > 1e-9 || math.Abs(*lng-want[1]) > 1e-9 {
			t.Errorf("parseCoord(%q) = %v, %v", in, lat, lng)
		}
	}
	for _, in := range []string{"", "không có", "0, 0", "123.4, 300.1"} {
		if lat, _ := parseCoord(in); lat != nil {
			t.Errorf("parseCoord(%q) phải rỗng", in)
		}
	}
}

func TestStageOf(t *testing.T) {
	for in, want := range map[string]string{
		"Hoàn thiện cơ bản": model.Stages[6], "Đang làm móng": model.Stages[2], "Đổ sàn tầng 2": model.Stages[3],
		"Xây thô": model.Stages[3], "Đã hoàn công": model.Stages[7], "Ép cọc": model.Stages[1], "Đổ mái": model.Stages[4],
		"Đi điện nước": model.Stages[5], "Dọn mặt bằng": model.Stages[0], "": "", "không rõ": "",
	} {
		if got := stageOf(in); got != want {
			t.Errorf("stageOf(%q) = %q, muốn %q", in, got, want)
		}
	}
}

func TestBankOf(t *testing.T) {
	if got := bankOf("Vietcombank 0123 456 789 Trần Văn A"); got != BankFrom("Vietcombank", "0123456789", "TRẦN VĂN A") {
		t.Errorf("bankOf tách được = %q", got)
	}
	if got := bankOf("stk 12345678 ngân hàng lạ"); got != "stk 12345678 ngân hàng lạ" {
		t.Errorf("không tách được thì giữ nguyên chữ: %q", got)
	}
	if bankOf("") != "" {
		t.Error("rỗng")
	}
}

func TestNormPhone(t *testing.T) {
	for in, want := range map[string]string{"+84901111111": "0901111111", "84901111111": "0901111111",
		"0943.488.882": "0943488882", " 0901 234 567 ": "0901234567"} {
		if got := normPhone(in); got != want {
			t.Errorf("normPhone(%q) = %q", in, got)
		}
	}
}
