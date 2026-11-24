package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// Hồ sơ năng lực công khai của nhà thầu (thiết kế "Dự án": nút Chia sẻ, 24/11/2026): link /nt/<mã>, chủ nhà mở xem
// công ty + các dự án (ảnh bìa, tình trạng, tiến độ), bấm dự án xem báo cáo nhật ký mới nhất (/bc/<mã>).
// Mã = id nhà thầu + chữ ký (không đoán được). Mỗi lượt mở cộng "Lượt xem" của hồ sơ.

func (s *NTService) portfolioSig(uid int64) string {
	m := hmac.New(sha256.New, s.mediaKey)
	fmt.Fprintf(m, "pf.%d", uid)
	return hex.EncodeToString(m.Sum(nil))[:12]
}

func (s *NTService) portfolioUser(code string) (int64, bool) {
	id, sig, ok := strings.Cut(code, "-")
	n, err := strconv.ParseInt(id, 10, 64)
	return n, ok && err == nil && n > 0 && hmac.Equal([]byte(sig), []byte(s.portfolioSig(n)))
}

// Portfolio: link hồ sơ năng lực của nhà thầu đang đăng nhập.
func (s *NTService) Portfolio(_ context.Context, u *model.User) *NTReportLink {
	c := fmt.Sprintf("%d-%s", u.ID, s.portfolioSig(u.ID))
	return &NTReportLink{Code: c, Path: "/nt/" + c}
}

type NTPublicProject struct {
	Name     string     `json:"name"`
	Address  string     `json:"address"`
	Kind     string     `json:"kind"`
	Status   string     `json:"status"`
	Progress int        `json:"progress"`
	Days     int        `json:"days"`
	Logs     int        `json:"logs"`
	Photos   int        `json:"photos"`
	Finished *time.Time `json:"finished_on"`
	CoverURL string     `json:"cover_url,omitempty"`
	Report   string     `json:"report,omitempty"` // /bc/<mã> của nhật ký mới nhất
}

type NTPublicPortfolio struct {
	Company     string            `json:"company"`
	Regions     string            `json:"regions"`
	Specialties []string          `json:"specialties"`
	Years       int               `json:"years"`
	Verified    bool              `json:"verified"`
	Phone       string            `json:"phone"`
	Views       int               `json:"views"`
	Projects    []NTPublicProject `json:"projects"`
	Logs        int               `json:"logs"`
}

func (s *NTService) PublicPortfolio(ctx context.Context, code string) (*NTPublicPortfolio, error) {
	uid, ok := s.portfolioUser(code)
	if !ok {
		return nil, fail(ErrNotFound, "Link hồ sơ không đúng.")
	}
	phone, err := s.repo.UserPhone(ctx, uid)
	if err != nil {
		return nil, err
	}
	if phone == "" {
		return nil, fail(ErrNotFound, "Hồ sơ này không còn.")
	}
	_ = s.repo.AddView(ctx, uid)
	prof, err := s.repo.Profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	ps, err := s.repo.Projects(ctx, uid, "")
	if err != nil {
		return nil, err
	}
	out := &NTPublicPortfolio{Company: prof.Company, Regions: prof.Regions, Specialties: prof.Specialties, Years: prof.Years,
		Verified: prof.Verified, Phone: phone, Views: prof.Views, Projects: []NTPublicProject{}}
	if out.Specialties == nil {
		out.Specialties = []string{}
	}
	s.covers(ps)
	for _, p := range ps {
		end := s.now()
		if p.FinishedOn != nil {
			end = *p.FinishedOn
		}
		x := NTPublicProject{Name: p.Name, Address: p.Address, Kind: p.Kind, Status: p.Status, Progress: p.Progress,
			Days: max(1, int(end.Sub(p.StartedOn).Hours()/24)+1), Logs: p.Logs, Photos: p.Photos, Finished: p.FinishedOn, CoverURL: p.CoverURL}
		if p.Status == NTStatusDone {
			x.Progress = 100
		}
		if p.Cover != nil {
			x.CoverURL = fmt.Sprintf("/api/v1/public/nt/%s/photos/%d", code, *p.Cover)
		}
		if logs, err := s.repo.Logs(ctx, p.ID); err == nil && len(logs) > 0 {
			x.Report = "/bc/" + s.ReportCode(logs[0].ID)
		}
		out.Logs += p.Logs
		out.Projects = append(out.Projects, x)
	}
	return out, nil
}

// PublicPortfolioPhoto: ảnh bìa dự án trên hồ sơ năng lực (chỉ ảnh thuộc dự án của đúng nhà thầu trong mã).
func (s *NTService) PublicPortfolioPhoto(ctx context.Context, code string, mediaID int64, small bool) ([]byte, error) {
	uid, ok := s.portfolioUser(code)
	if !ok {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	m, err := s.repo.Media(ctx, mediaID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && m.Kind != "photo") {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if err != nil {
		return nil, err
	}
	p, err := s.repo.Project(ctx, m.ProjectID)
	if err != nil || p.UserID != uid {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	b, err := s.files.ReadPhoto(m.SHA256, small)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file ảnh.")
	}
	return b, nil
}
