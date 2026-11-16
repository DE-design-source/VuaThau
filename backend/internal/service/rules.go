package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"vuathau/backend/internal/model"
)

// Luật kiểm ảnh nghiệm thu (như photo.py của bản cũ).
const (
	MaxAgeDays    = 30 // ảnh chụp quá số ngày này bị loại
	ClockSkewMin  = 60 // giờ in trên ảnh lệch EXIF quá số phút này: nghi sửa ảnh
	DupHamming    = 6  // dHash khác <= 6/64 bit: cùng một ảnh
	DupHouseM     = 30 // nhà cách nhà đã nộp dưới 30m: gắn cờ cho admin
	MaxFileBytes  = 20 << 20
	SmallSide     = 1568 // cạnh dài ảnh thu nhỏ (xem trên web/app, gửi AI)
	ReceiptSide   = 2000
	PageSize      = 50
	ListLimit     = 200
	MaxReasonLen  = 500
	MaxAddressLen = 300
)

// VN: múi giờ Việt Nam, dùng cho ngày giờ chụp và báo cáo.
var VN = func() *time.Location {
	if l, err := time.LoadLocation("Asia/Ho_Chi_Minh"); err == nil {
		return l
	}
	return time.FixedZone("ICT", 7*3600)
}()

// Lý do mẫu khi admin trả lại / từ chối.
var Reasons = map[string][]string{
	model.StatusReturned: {"Ảnh mờ, chụp lại giúp", "Thiếu ảnh bảng hiệu nhà thầu", "Sai SĐT nhà thầu", "Sai địa chỉ công trình"},
	model.StatusRejected: {"Không phải công trình đang xây", "Trùng nhà đã nộp", "Ảnh chụp lại màn hình"},
}

var Banks = []string{"Vietcombank", "VietinBank", "BIDV", "Agribank", "Techcombank", "MB Bank", "ACB", "VPBank", "Sacombank",
	"TPBank", "VIB", "HDBank", "SHB", "OCB", "MSB", "SeABank", "Eximbank", "LPBank", "Nam A Bank", "Cake", "Timo"}

var (
	phoneRE    = regexp.MustCompile(`^(0|\+84)\d{9,10}$`)
	staffLogin = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)
	dayRE      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	minuteRE   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$`)
	bankNoRE   = regexp.MustCompile(`^\d{6,20}$`)
	spacesRE   = regexp.MustCompile(`[\s.\-]`)
	digitsRE   = regexp.MustCompile(`\d[\d .-]{4,}\d`)
	nonDigitRE = regexp.MustCompile(`\D`)
)

// CleanPhone bỏ dấu cách, chấm, gạch trong SĐT.
func CleanPhone(s string) string { return spacesRE.ReplaceAllString(s, "") }

// LoginID: tên đăng nhập chuẩn hoá (SĐT hoặc tên nhân viên viết thường).
func LoginID(s string) string { return strings.ToLower(CleanPhone(strings.TrimSpace(s))) }

func ValidPhone(s string) bool { return phoneRE.MatchString(s) }

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// BankFrom ghép 3 ô ngân hàng thành 1 chuỗi; "" nếu thiếu hoặc số TK sai.
func BankFrom(name, no, owner string) string {
	name, owner = strings.TrimSpace(name), strings.TrimSpace(owner)
	no = spacesRE.ReplaceAllString(no, "")
	if name == "" || owner == "" || !bankNoRE.MatchString(no) {
		return ""
	}
	return fmt.Sprintf("%s · %s · %s", truncate(name, 60), no, truncate(strings.ToUpper(owner), 80))
}

// BankParts tách chuỗi ngân hàng. Dữ liệu cũ gõ tự do: đoán số TK là dãy số dài nhất.
func BankParts(s string) model.Bank {
	parts := strings.Split(s, "·")
	if len(parts) == 3 {
		return model.Bank{Name: strings.TrimSpace(parts[0]), No: strings.TrimSpace(parts[1]), Owner: strings.TrimSpace(parts[2])}
	}
	b := model.Bank{Name: s}
	best := ""
	for _, m := range digitsRE.FindAllString(s, -1) {
		if len(m) > len(best) {
			best = m
		}
	}
	b.No = nonDigitRE.ReplaceAllString(best, "")
	return b
}

func parseMinute(s string) (time.Time, bool) {
	if !minuteRE.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", s, VN)
	return t, err == nil
}

// PhotoProblems: lý do ảnh không được tính tiền. Rỗng = ảnh hợp lệ.
func PhotoProblems(p *model.Photo, now time.Time) []string {
	out := []string{}
	et, okE := parseMinute(p.ExifTime)
	at, okA := parseMinute(p.AITime)
	t, ok := et, okE
	if !ok {
		t, ok = at, okA
	}
	switch {
	case !ok:
		out = append(out, "Không đọc được thời gian chụp")
	case now.Sub(t) > MaxAgeDays*24*time.Hour:
		out = append(out, fmt.Sprintf("Ảnh chụp quá %d ngày", MaxAgeDays))
	case t.Sub(now) > time.Hour:
		out = append(out, "Thời gian chụp ở tương lai")
	}
	if okE && okA {
		d := et.Sub(at)
		if d < 0 {
			d = -d
		}
		if d > ClockSkewMin*time.Minute {
			out = append(out, "Giờ in trên ảnh lệch giờ chụp thật")
		}
	}
	if p.Lat == nil && strings.TrimSpace(p.Address) == "" {
		out = append(out, "Không có vị trí")
	}
	if p.IsSite != nil && !*p.IsSite {
		out = append(out, "Không thấy công trình đang xây")
	}
	if p.DupOf != nil {
		out = append(out, fmt.Sprintf("Trùng ảnh đã nộp (hồ sơ #%d)", *p.DupOf))
	}
	return out
}

// FillProblems tính Problems cho từng ảnh, trả số ảnh hợp lệ.
func FillProblems(ps []model.Photo, now time.Time) int {
	valid := 0
	for i := range ps {
		ps[i].Problems = PhotoProblems(&ps[i], now)
		if len(ps[i].Problems) == 0 {
			valid++
		}
	}
	return valid
}

// CheckItem: 1 điều kiện gửi duyệt.
type CheckItem struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// Checklist: điều kiện để CTV gửi duyệt hồ sơ.
func Checklist(h *model.House, valid, minPhotos int) []CheckItem {
	items := []CheckItem{{
		Key: "anh", Label: fmt.Sprintf("Ít nhất %d ảnh hợp lệ", minPhotos), OK: valid >= minPhotos,
		Message: fmt.Sprintf("Cần ít nhất %d ảnh hợp lệ (đang có %d)", minPhotos, valid),
	}}
	for _, f := range model.Fields {
		v := h.Value(f.Key)
		badPhone := strings.HasSuffix(f.Key, "phone") && v != "" && !ValidPhone(v)
		msg := "Thiếu " + f.Label
		if badPhone {
			msg = capFirst(f.Label) + " không hợp lệ"
		}
		items = append(items, CheckItem{Key: f.Key, Label: capFirst(f.Label), OK: v != "" && !badPhone, Message: msg})
	}
	return items
}

// stepOf: số bước đã xong của trạng thái (trên 4 bước).
var stepOf = map[string]int{
	model.StatusDraft: 1, model.StatusReturned: 1, model.StatusPending: 2, model.StatusRejected: 2,
	model.StatusApproved: 3, model.StatusPaid: 4,
}

// Steps: tiến trình 4 bước. approved: số hồ sơ đã duyệt chờ trả của CTV.
func Steps(h *model.House, logs []model.Log, approved, payMin int) []model.Step {
	last := func(acts ...string) string {
		for i := len(logs) - 1; i >= 0; i-- {
			for _, a := range acts {
				if logs[i].Action == a {
					return logs[i].At.In(VN).Format(time.RFC3339)
				}
			}
		}
		return ""
	}
	aiHint := "Xem phản hồi bên dưới, bổ sung đủ rồi bấm Gửi duyệt."
	if h.AIState == model.AIReading {
		aiHint = "AI đang đọc ảnh…"
	} else if h.Status == model.StatusReturned {
		aiHint = "Admin trả lại hồ sơ. Sửa theo lý do ở đầu trang rồi gửi lại."
	}
	payHint := fmt.Sprintf("Kế toán thanh toán khi đủ %d hồ sơ đã duyệt (đang có %d).", payMin, approved)
	if approved >= payMin {
		payHint = fmt.Sprintf("Đủ %d hồ sơ đã duyệt, kế toán sẽ chuyển khoản sớm.", payMin)
	}
	defs := []struct{ title, who, at, hint string }{
		{"Tải ảnh lên", "Cộng tác viên", last(ActCreate), ""},
		{"AI kiểm tra, gửi duyệt", "AI + Cộng tác viên", last(ActSubmit), aiHint},
		{"Admin duyệt", "Admin", last(ActApprove, ActReject), "Admin đang xem ảnh và thông tin."},
		{"Thanh toán", "Kế toán", last(ActPaid), payHint},
	}
	n := stepOf[h.Status]
	out := make([]model.Step, 0, 4)
	for i, d := range defs {
		state := "todo"
		if i < n {
			state = "done"
		} else if i == n {
			state = "current"
		}
		hint := d.hint
		if h.Status == model.StatusRejected && i == 2 {
			state, hint = "bad", "Hồ sơ bị từ chối, không được thanh toán."
		} else if h.Status == model.StatusRejected && i > 2 {
			state = "todo"
		}
		s := model.Step{Title: d.title, Who: d.who, State: state}
		if state == "done" || state == "bad" {
			s.At = d.at
		}
		if state == "current" || state == "bad" {
			s.Hint = hint
		}
		out = append(out, s)
	}
	return out
}

// Tên thao tác ghi vào lịch sử (dùng để dựng tiến trình, báo cáo).
const (
	ActCreate   = "Tạo hồ sơ"
	ActSubmit   = "Gửi duyệt"
	ActApprove  = "Duyệt"
	ActReturn   = "Trả lại cho CTV sửa"
	ActReject   = "Từ chối"
	ActPaid     = "Đã thanh toán"
	ActUndo     = "Hoàn tác, đưa về chờ duyệt"
	ActEdit     = "Admin sửa thông tin"
	ActAmount   = "Sửa số tiền"
	ActCancel   = "Huỷ phiếu chi"
	ActUnpay    = "Huỷ thanh toán"
	ActReceipt  = "Thêm ảnh biên lai"
	ActRef      = "Sửa mã giao dịch"
	ActAdjust   = "Thưởng / trừ tiền"
	ActAdjDel   = "Xoá thưởng / trừ"
	ActLock     = "Khoá tài khoản"
	ActUnlock   = "Mở khoá tài khoản"
	ActResetPW  = "Đặt lại mật khẩu"
	ActAddStaff = "Thêm nhân viên"
	ActAddCTV   = "Admin tạo tài khoản CTV"
	ActSettings = "Sửa cấu hình"
	ActSelfDel  = "CTV tự xoá tài khoản"
)

// Money định dạng "35.000đ".
func Money(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String() + "đ"
	}
	return b.String() + "đ"
}

// CleanFields làm sạch ô thông tin công trình.
func CleanFields(f model.HouseFields) model.HouseFields {
	out := model.HouseFields{
		Address:         truncate(f.Address, MaxAddressLen),
		ContractorName:  truncate(f.ContractorName, 200),
		ContractorPhone: CleanPhone(strings.TrimSpace(f.ContractorPhone)),
	}
	if f.Floors != nil && *f.Floors > 0 && *f.Floors < 200 {
		v := *f.Floors
		out.Floors = &v
	}
	for _, s := range model.Stages {
		if f.Stage == s {
			out.Stage = s
		}
	}
	return out
}

// ValidDay: chuỗi "YYYY-MM-DD".
func ValidDay(s string) bool { return dayRE.MatchString(s) }
