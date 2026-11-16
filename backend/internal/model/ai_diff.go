package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// AIFieldKeys: các ô so giá trị đang lưu với giá trị AI đọc trên ảnh.
// Không so địa chỉ: dải chữ Timemark ghi địa chỉ theo kiểu riêng, lệch cách viết là chuyện thường.
var AIFieldKeys = []string{"contractor_name", "contractor_phone", "floors", "stage"}

// FillAIDiff ghi vào AIDiff các ô mà giá trị đang lưu khác giá trị AI đọc trên ảnh
// (CTV / chatbot khai khác, hoặc admin đã sửa). Khác cách viết thì không tính:
// hoa thường, dấu, "Công ty TNHH ..." bao tên ngắn, +84 / 0, một ô ghi 2 SĐT có chứa số AI đọc.
func (h *House) FillAIDiff() {
	h.AIDiff = nil
	if len(h.AIJSON) == 0 {
		return
	}
	var got map[string]any
	if json.Unmarshal(h.AIJSON, &got) != nil {
		return
	}
	str := func(k string) string {
		switch v := got[k].(type) {
		case string:
			return strings.TrimSpace(v)
		case float64:
			if v > 0 {
				return strconv.Itoa(int(v))
			}
		}
		return ""
	}
	out := map[string]string{}
	for _, k := range AIFieldKeys {
		ai, cur := str(k), h.Value(k)
		if ai == "" || cur == "" || sameAIValue(k, cur, ai) {
			continue
		}
		out[k] = ai
	}
	if len(out) > 0 {
		h.AIDiff = out
	}
}

func sameAIValue(key, cur, ai string) bool {
	switch key {
	case "contractor_phone":
		c, a := phoneDigits(cur), phoneDigits(ai)
		return a == "" || strings.Contains(c, a)
	case "contractor_name":
		c, a := foldName(cur), foldName(ai)
		return c == "" || a == "" || strings.Contains(c, a) || strings.Contains(a, c)
	}
	return cur == ai
}

// phoneDigits: chỉ giữ chữ số, +84 / 84 đầu số đổi thành 0.
func phoneDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if strings.HasPrefix(strings.TrimSpace(s), "+84") || (strings.HasPrefix(d, "84") && len(d) == 11) {
		d = "0" + d[2:]
	}
	return d
}

var unaccent = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// foldName: bỏ dấu, chữ thường, chỉ giữ chữ và số (đ → d).
func foldName(s string) string {
	s, _, _ = transform.String(unaccent, strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 'đ':
			b.WriteRune('d')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}
