package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"vuathau/backend/internal/integration/ai"
	"vuathau/backend/internal/model"
)

// Quy tắc duyệt ảnh cho AI (góp ý 21/11/2026): admin tự viết ở trang Cấu hình, lưu ở sync_state (khoá ai_rules),
// gửi kèm sau lời dặn mặc định mỗi lượt AI đọc ảnh. Lời dặn mặc định giữ nguyên (khung JSON, cách đọc giờ / địa chỉ),
// nên quy tắc viết sai cũng không làm hỏng việc đọc kết quả.

const aiRulesKey = "ai_rules"

type AIRules struct {
	Rules         string     `json:"rules"`
	At            *time.Time `json:"at"`             // lần sửa gần nhất
	DefaultPrompt string     `json:"default_prompt"` // lời dặn mặc định (tham khảo)
	PhotoOrder    string     `json:"photo_order"`    // quy tắc xếp thứ tự ảnh công trình (PhotoOrders)
	Max           int        `json:"max"`
}

// nhớ tạm 30 giây: mỗi lượt đọc ảnh không phải hỏi CSDL
type aiRulesCache struct {
	mu sync.Mutex
	v  string
	at time.Time
}

func (s *houseService) aiRules(ctx context.Context) string {
	if s.sync == nil {
		return ""
	}
	s.rules.mu.Lock()
	defer s.rules.mu.Unlock()
	if !s.rules.at.IsZero() && time.Since(s.rules.at) < 30*time.Second {
		return s.rules.v
	}
	v, _, err := s.sync.Get(ctx, aiRulesKey)
	if err != nil {
		return s.rules.v // CSDL lỗi: dùng bản cũ
	}
	s.rules.v, s.rules.at = v, time.Now()
	return v
}

func (s *houseService) AIRules(ctx context.Context) (*AIRules, error) {
	res := &AIRules{DefaultPrompt: ai.DefaultPrompt(), Max: ai.MaxRules}
	if s.sync == nil {
		return res, nil
	}
	v, at, err := s.sync.Get(ctx, aiRulesKey)
	if err != nil {
		return nil, err
	}
	res.Rules = v
	res.PhotoOrder = s.photoOrder(ctx)
	if !at.IsZero() {
		res.At = &at
	}
	return res, nil
}

func (s *houseService) SaveAIRules(ctx context.Context, by *model.User, rules string) (string, *AIRules, error) {
	rules = strings.TrimSpace(strings.ReplaceAll(rules, "\r\n", "\n"))
	if utf8.RuneCountInString(rules) > ai.MaxRules {
		return "", nil, fail(ErrInvalidInput, "Chưa lưu: quy tắc dài quá %d ký tự.", ai.MaxRules)
	}
	if s.sync == nil {
		return "", nil, fail(ErrUnavailable, "Máy chủ chưa bật lưu quy tắc.")
	}
	old, _, err := s.sync.Get(ctx, aiRulesKey)
	if err != nil {
		return "", nil, err
	}
	if old == rules {
		res, err := s.AIRules(ctx)
		return "Không có gì thay đổi.", res, err
	}
	if err := s.sync.Set(ctx, aiRulesKey, rules); err != nil {
		return "", nil, err
	}
	s.rules.mu.Lock()
	s.rules.v, s.rules.at = rules, time.Now()
	s.rules.mu.Unlock()
	_ = s.logs.Add(ctx, by.ID, ActAIRules, nil, nil, fmt.Sprintf("%d -> %d ký tự", utf8.RuneCountInString(old), utf8.RuneCountInString(rules)))
	res, err := s.AIRules(ctx)
	msg := "Đã lưu quy tắc. AI dùng từ lượt đọc ảnh tiếp theo."
	if rules == "" {
		msg = "Đã xoá quy tắc thêm. AI dùng lời dặn mặc định."
	}
	return msg, res, err
}

// Quy tắc xếp thứ tự ảnh công trình (góp ý 23/11/2026, trang Cấu hình): áp dụng khi xem hồ sơ (admin, kế toán),
// dự án của sale và trang công khai; ảnh đầu tiên là ảnh đại diện.
const photoOrderKey = "photo_order"

// PhotoOrders: mã -> mô tả (trang Cấu hình hiện theo thứ tự PhotoOrderList).
var PhotoOrders = map[string]string{
	"cover_first": "Ảnh đẹp nhất (AI chọn: toàn cảnh, rõ nét, đúng chiều) làm ảnh đại diện, còn lại bảng hiệu trước rồi theo giờ chụp",
	"sign_first":  "Ảnh bảng hiệu (có SĐT nhà thầu) lên đầu, còn lại theo giờ chụp",
	"time":        "Theo giờ chụp, cũ trước mới sau",
	"sent":        "Theo thứ tự CTV gửi",
}
var PhotoOrderList = []string{"cover_first", "sign_first", "time", "sent"}

const defaultPhotoOrder = "cover_first" // góp ý Lark 26/11/2026: ảnh đại diện phải là ảnh đẹp, toàn cảnh

func (s *houseService) photoOrder(ctx context.Context) string {
	if s.sync == nil {
		return defaultPhotoOrder
	}
	v, _, err := s.sync.Get(ctx, photoOrderKey)
	if _, ok := PhotoOrders[v]; err != nil || !ok {
		return defaultPhotoOrder
	}
	return v
}

func (s *houseService) SavePhotoOrder(ctx context.Context, by *model.User, order string) (string, error) {
	if _, ok := PhotoOrders[order]; !ok {
		return "", fail(ErrInvalidInput, "Quy tắc xếp ảnh không hợp lệ.")
	}
	if s.sync == nil {
		return "", fail(ErrUnavailable, "Máy chủ chưa bật lưu cấu hình.")
	}
	if err := s.sync.Set(ctx, photoOrderKey, order); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActAIRules, nil, nil, "Xếp ảnh: "+PhotoOrders[order])
	return "Đã lưu quy tắc xếp ảnh: " + PhotoOrders[order] + ".", nil
}

// photoTime: giờ chụp để xếp (EXIF, không có thì AI đọc trên ảnh); "" xếp sau cùng.
func photoTime(p model.Photo) string {
	if p.ExifTime != "" {
		return p.ExifTime
	}
	return p.AITime
}

// sortPhotos: xếp ảnh theo quy tắc (giữ nguyên thứ tự gửi khi bằng nhau).
func sortPhotos(ps []model.Photo, order string) {
	if order == "sent" {
		return
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if order == "sign_first" || order == "cover_first" {
			si, sj := ps[i].Masked > 0, ps[j].Masked > 0
			if si != sj {
				return si
			}
		}
		ti, tj := photoTime(ps[i]), photoTime(ps[j])
		if (ti == "") != (tj == "") {
			return tj == ""
		}
		return ti < tj
	})
	if order == "cover_first" && len(ps) > 1 {
		if b := bestCover(ps); b > 0 {
			c := ps[b]
			copy(ps[1:b+1], ps[:b])
			ps[0] = c
		}
	}
}

// bestCover: vị trí ảnh hợp làm ảnh đại diện nhất (AI chấm cao nhất); chưa chấm / bằng nhau thì ảnh đầu.
func bestCover(ps []model.Photo) int {
	b := 0
	for i, p := range ps {
		if p.CoverScore > ps[b].CoverScore {
			b = i
		}
	}
	return b
}
