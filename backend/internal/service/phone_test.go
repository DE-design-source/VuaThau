package service

import "testing"

func TestFirstPhone(t *testing.T) {
	cases := map[string]string{
		// giá trị thật chatbot "Cập nhật công trình" đã ghi
		"0933.547.039 - 0922.121.030": "0933547039",
		"09 45678 300 - 09 3979 3908": "0945678300",
		"0945678300 / 0939793908":     "0945678300",
		"0903 883 848 - 0907 244 432": "0903883848",
		"0986972232 - 0368499499":     "0986972232",
		// 1 số: giữ như cũ
		"0933-547-039":    "0933547039",
		"+84 975 067 595": "0975067595",
		"0913197706":      "0913197706",
		// khác
		"0911222333, 0988777666":          "0911222333",
		"0911222333 hoặc 0988777666":      "0911222333",
		"09112223330988777666":            "0911222333", // dính liền không dấu ngăn cách
		"hotline: 1900 1234 - 0911222333": "0911222333", // số đầu không hợp lệ: lấy số hợp lệ kế tiếp
		"":                                "",
		"không có":                        "khôngcó",
	}
	for in, want := range cases {
		if got := firstPhone(in); got != want {
			t.Errorf("firstPhone(%q) = %q, muốn %q", in, got, want)
		}
	}
}
