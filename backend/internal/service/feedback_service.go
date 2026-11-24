package service

import (
	"context"
	"strings"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// Đánh giá app (góp ý 25/11/2026): CTV / nhà thầu tải lên đủ FeedbackEvery ảnh thì app hỏi 1-5 sao + góp ý.
// Bấm "Để sau": hỏi lại 1 lần khi đủ gấp đôi số ảnh. Đã đánh giá: không hỏi nữa.

const (
	FeedbackEvery     = 30
	feedbackMaxLaters = 1 // số lần "Để sau" còn được hỏi lại
	maxFeedbackLen    = 2000
)

type FeedbackService struct{ repo repository.FeedbackRepository }

func NewFeedbackService(repo repository.FeedbackRepository) *FeedbackService {
	return &FeedbackService{repo: repo}
}

// FeedbackAsk: app có nên hỏi đánh giá lúc này không.
type FeedbackAsk struct {
	Ask    bool `json:"ask"`
	Photos int  `json:"photos"`
}

func appUser(u *model.User) bool { return u.Role == model.RoleCTV || u.Role == model.RoleNhaThau }

func (s *FeedbackService) Ask(ctx context.Context, u *model.User) (*FeedbackAsk, error) {
	if !appUser(u) {
		return &FeedbackAsk{}, nil
	}
	st, err := s.repo.State(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if st.Rated || st.Laters > feedbackMaxLaters {
		return &FeedbackAsk{}, nil
	}
	n, err := s.repo.Photos(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &FeedbackAsk{Ask: n >= FeedbackEvery*(st.Laters+1), Photos: n}, nil
}

type FeedbackInput struct {
	Stars    int    `json:"stars"`
	Comment  string `json:"comment"`
	Later    bool   `json:"later"` // bấm "Để sau"
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

func (s *FeedbackService) Submit(ctx context.Context, u *model.User, in FeedbackInput) (string, error) {
	if !appUser(u) {
		return "", ErrForbidden
	}
	f := &repository.Feedback{UserID: u.ID, Later: in.Later, Version: truncate(in.Version, 40), Platform: truncate(in.Platform, 20),
		Comment: truncate(strings.ToValidUTF8(strings.TrimSpace(in.Comment), ""), maxFeedbackLen)}
	if !in.Later {
		if in.Stars < 1 || in.Stars > 5 {
			return "", fail(ErrInvalidInput, "Chọn từ 1 đến 5 sao nhé.")
		}
		f.Stars = &in.Stars
	} else {
		f.Comment = ""
	}
	if n, err := s.repo.Photos(ctx, u.ID); err == nil {
		f.Photos = n
	}
	if err := s.repo.Add(ctx, f); err != nil {
		return "", err
	}
	if in.Later {
		return "", nil
	}
	return "Cảm ơn bạn đã đánh giá! Vua Thầu sẽ đọc từng góp ý.", nil
}

// FeedbackList: trang Đánh giá app (admin).
type FeedbackList struct {
	Items []repository.Feedback `json:"items"`
	Count int                   `json:"count"`
	Avg   float64               `json:"avg"`   // số sao trung bình
	Stars [6]int                `json:"stars"` // stars[k]: số lượt k sao
}

func (s *FeedbackService) List(ctx context.Context) (*FeedbackList, error) {
	items, err := s.repo.List(ctx, 1000)
	if err != nil {
		return nil, err
	}
	out := &FeedbackList{Items: items, Count: len(items)}
	sum := 0
	for _, f := range items {
		if f.Stars != nil {
			out.Stars[*f.Stars]++
			sum += *f.Stars
		}
	}
	if out.Count > 0 {
		out.Avg = float64(sum) / float64(out.Count)
	}
	return out, nil
}
