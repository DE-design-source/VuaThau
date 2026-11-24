package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // logo JPG
	_ "image/png"  // logo PNG nền trong
	"math"
	"sort"
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
	// nhà thầu bị khoá (admin, bỏ xác minh trên Lark) hoặc đã xoá: trang công khai thôi hiện
	phone, err := s.repo.PublicPhone(ctx, uid)
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
	if err != nil || p.UserID != uid || !s.publicOpen(ctx, uid) {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	b, err := s.files.ReadPhoto(m.SHA256, small)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file ảnh.")
	}
	return b, nil
}

// ---------- Logo trên ảnh ----------

type NTBrandingInput struct {
	Brand string `json:"brand"` // tên thương hiệu in trên ảnh khi chưa có logo (tối đa 20 ký tự)
	Size  int    `json:"size"`  // 1 nhỏ, 2 vừa, 3 lớn
	Badge bool   `json:"badge"` // giữ nhãn "Vua Thầu xác minh"
}

const maxBrandRunes = 20

func (s *NTService) SaveBranding(ctx context.Context, u *model.User, in NTBrandingInput) (*repository.NTProfile, error) {
	brand := strings.TrimSpace(in.Brand)
	if r := []rune(brand); len(r) > maxBrandRunes {
		return nil, fail(ErrInvalidInput, "Tên thương hiệu tối đa %d ký tự.", maxBrandRunes)
	}
	size := min(max(in.Size, 1), 3)
	if err := s.repo.SaveBranding(ctx, u.ID, brand, "-", size, in.Badge); err != nil {
		return nil, err
	}
	return s.repo.Profile(ctx, u.ID)
}

// maxLogoBytes: logo công ty tải lên (PNG nền trong / JPG).
const maxLogoBytes = 5 << 20

// SaveLogo: lưu logo công ty (giữ nguyên file, PNG nền trong vẫn trong). data nil: xoá logo.
func (s *NTService) SaveLogo(ctx context.Context, u *model.User, data []byte) (*repository.NTProfile, error) {
	prof, err := s.repo.Profile(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	sha := ""
	if data != nil {
		if len(data) > maxLogoBytes {
			return nil, fail(ErrInvalidInput, "Logo quá 5MB.")
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fail(ErrInvalidInput, "File logo không phải ảnh (PNG hoặc JPG).")
		}
		if cfg.Width < 100 || cfg.Height < 100 {
			return nil, fail(ErrInvalidInput, "Logo nhỏ quá (%dx%d), cần tối thiểu 100×100px.", cfg.Width, cfg.Height)
		}
		// app giải nén logo để in lên mọi ảnh: logo khổng lồ (PNG nén nhỏ) làm app hết bộ nhớ
		if cfg.Width > 4096 || cfg.Height > 4096 {
			return nil, fail(ErrInvalidInput, "Logo lớn quá (%dx%d), tối đa 4096×4096px.", cfg.Width, cfg.Height)
		}
		sum := sha256.Sum256(data)
		sha = hex.EncodeToString(sum[:])
		if err := s.files.SavePhoto(sha, data, nil); err != nil {
			return nil, err
		}
	}
	if err := s.repo.SaveBranding(ctx, u.ID, prof.Brand, sha, max(prof.LogoSize, 1), prof.ShowBadge); err != nil {
		return nil, err
	}
	return s.repo.Profile(ctx, u.ID)
}

// Logo: file logo của nhà thầu (app tải về để in lên ảnh); chưa có: 404.
func (s *NTService) Logo(ctx context.Context, u *model.User) ([]byte, error) {
	prof, err := s.repo.Profile(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if prof.LogoSHA == "" {
		return nil, fail(ErrNotFound, "Chưa có logo.")
	}
	b, err := s.files.ReadPhoto(prof.LogoSHA, false)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file logo.")
	}
	return b, nil
}

// ---------- Khám phá (thiết kế "App – Khám phá") ----------
// Bảng tin hoạt động thi công gần đây của các nhà thầu trên Vua Thầu: ai đang thi công ở đâu, ảnh mới nhất,
// bấm vào xem hồ sơ năng lực. Lọc: Tất cả / Gần tôi (≤ 10 km) / Đã xác minh / Tổng thầu / Thầu phụ / Tổ đội; tìm theo
// tên công ty, dự án, địa chỉ.

type NTExploreItem struct {
	LogID      int64     `json:"log_id"`
	Company    string    `json:"company"`
	Verified   bool      `json:"verified"`
	Kind       string    `json:"kind,omitempty"`
	Role       string    `json:"role,omitempty"`
	Staff      string    `json:"staff,omitempty"`
	Phone      string    `json:"phone"`
	Project    string    `json:"project"`
	Address    string    `json:"address"`
	Note       string    `json:"note"`
	TakenAt    time.Time `json:"taken_at"`
	Lat        *float64  `json:"lat,omitempty"`
	Lng        *float64  `json:"lng,omitempty"`
	DistanceKm *float64  `json:"distance_km,omitempty"`
	Photos     []string  `json:"photos"`
	PhotoCount int       `json:"photo_count"`
	Portfolio  string    `json:"portfolio"` // /nt/<mã>
	Mine       bool      `json:"mine"`
}

type NTExploreQuery struct {
	Q        string
	Filter   string // all | near | verified | tong_thau | thau_phu | to_doi
	Lat, Lng *float64
}

const exploreNearKm = 10

func (s *NTService) Explore(ctx context.Context, u *model.User, in NTExploreQuery) ([]NTExploreItem, error) {
	rows, err := s.repo.Explore(ctx, time.Now().AddDate(0, 0, -60), 400)
	if err != nil {
		return nil, err
	}
	q := model.FoldName(strings.TrimSpace(in.Q))
	out := []NTExploreItem{}
	seen := map[string]bool{} // mỗi dự án chỉ hiện nhật ký mới nhất
	for _, r := range rows {
		key := fmt.Sprintf("%d|%s", r.UserID, r.Project)
		if seen[key] {
			continue
		}
		company := r.Company
		if company == "" {
			company = r.Name
		}
		var dist *float64
		if in.Lat != nil && in.Lng != nil && r.Lat != nil && r.Lng != nil {
			d := haversineKm(*in.Lat, *in.Lng, *r.Lat, *r.Lng)
			dist = &d
		}
		kindRole := model.FoldName(r.Kind + " " + r.Role)
		switch in.Filter {
		case "near":
			if dist == nil || *dist > exploreNearKm {
				continue
			}
		case "verified":
			if !r.Verified {
				continue
			}
		case "tong_thau":
			if !strings.Contains(kindRole, "tongthau") {
				continue
			}
		case "thau_phu":
			if !strings.Contains(kindRole, "thauphu") {
				continue
			}
		case "to_doi":
			if !strings.Contains(kindRole, "todoi") {
				continue
			}
		}
		if q != "" && !strings.Contains(model.FoldName(company+" "+r.Project+" "+r.Address+" "+r.Note), q) {
			continue
		}
		seen[key] = true
		code := fmt.Sprintf("%d-%s", r.UserID, s.portfolioSig(r.UserID))
		photos := []string{}
		for i, id := range r.Photos {
			if i == 4 {
				break
			}
			photos = append(photos, fmt.Sprintf("/api/v1/public/nt/%s/photos/%d", code, id))
		}
		out = append(out, NTExploreItem{
			LogID: r.LogID, Company: company, Verified: r.Verified, Kind: r.Kind, Role: r.Role, Staff: r.Staff, Phone: r.Phone,
			Project: r.Project, Address: r.Address, Note: r.Note, TakenAt: r.TakenAt, Lat: r.Lat, Lng: r.Lng, DistanceKm: dist,
			Photos: photos, PhotoCount: len(r.Photos), Portfolio: "/nt/" + code, Mine: r.UserID == u.ID,
		})
		if len(out) == 60 {
			break
		}
	}
	if in.Filter == "near" {
		sort.SliceStable(out, func(i, j int) bool { return *out[i].DistanceKm < *out[j].DistanceKm })
	}
	return out, nil
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dlat, dlng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dlng/2)*math.Sin(dlng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
