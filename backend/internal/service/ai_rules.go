package service

import (
	"context"
	"fmt"
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
