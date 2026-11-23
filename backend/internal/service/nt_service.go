package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/storage"
)

// Nhà thầu trên app (thiết kế "App Vua Thầu Map – Nhà thầu", đợt 1, 23/11/2026):
//   - đăng nhập bằng SĐT (admin thêm tài khoản / form đăng ký nhà thầu trên Lark);
//   - Trang chủ: hồ sơ công ty + công trình của bạn (nhắc chưa cập nhật hôm nay);
//   - Dự án (hồ sơ năng lực): lọc đang thi công / hoàn thành; chi tiết: tiến độ, nhật ký thi công (ảnh / video có giờ + GPS);
//   - chụp nhật ký bằng camera Vua Thầu (in giờ, địa chỉ, GPS) rồi tải lên kèm mô tả việc đã làm.
// Nhà thầu chỉ thấy / sửa dự án của mình; admin xem mọi nhà thầu trên web.

const (
	NTStatusRunning = "dang_thi_cong"
	NTStatusDone    = "hoan_thanh"
)

type NTService struct {
	repo     repository.NTRepository
	auth     repository.AuthRepository
	files    *storage.Files
	logs     repository.LogRepository
	mediaKey []byte
	now      func() time.Time
}

func NewNTService(repo repository.NTRepository, auth repository.AuthRepository, files *storage.Files, logs repository.LogRepository, mediaKey string) *NTService {
	return &NTService{repo: repo, auth: auth, files: files, logs: logs, mediaKey: []byte("nt-media:" + mediaKey), now: time.Now}
}

func (s *NTService) mine(ctx context.Context, u *model.User, id int64) (*repository.NTProject, error) {
	p, err := s.repo.Project(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && u.Role == model.RoleNhaThau && p.UserID != u.ID) {
		return nil, fail(ErrNotFound, "Không tìm thấy dự án.")
	}
	if err != nil {
		return nil, err
	}
	if u.Role != model.RoleNhaThau && u.Role != model.RoleAdmin {
		return nil, ErrForbidden
	}
	return p, nil
}

// ---------- Trang chủ ----------

type NTHome struct {
	User     *model.User            `json:"user"`
	Profile  *repository.NTProfile  `json:"profile"`
	Projects []repository.NTProject `json:"projects"`
	Stats    struct {
		Projects int `json:"projects"`
		Running  int `json:"running"`
		Done     int `json:"done"`
		Logs     int `json:"logs"`
		Views    int `json:"views"`
	} `json:"stats"`
}

func (s *NTService) Home(ctx context.Context, u *model.User) (*NTHome, error) {
	prof, err := s.repo.Profile(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	ps, err := s.repo.Projects(ctx, u.ID, "")
	if err != nil {
		return nil, err
	}
	h := &NTHome{User: u, Profile: prof, Projects: ps}
	for _, p := range ps {
		h.Stats.Projects++
		h.Stats.Logs += p.Logs
		if p.Status == NTStatusDone {
			h.Stats.Done++
		} else {
			h.Stats.Running++
		}
	}
	h.Stats.Views = prof.Views
	return h, nil
}

// ---------- Hồ sơ công ty ----------

type NTProfileInput struct {
	Company     string   `json:"company"`
	Specialties []string `json:"specialties"`
	Regions     string   `json:"regions"`
	Years       int      `json:"years"`
}

func (s *NTService) SaveProfile(ctx context.Context, u *model.User, in NTProfileInput) (*repository.NTProfile, error) {
	cur, err := s.repo.Profile(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	cur.Company = truncate(in.Company, 200)
	cur.Regions = truncate(in.Regions, 200)
	cur.Years = min(max(in.Years, 0), 80)
	cur.Specialties = []string{}
	for _, x := range in.Specialties {
		if x = truncate(x, 40); x != "" && len(cur.Specialties) < 10 {
			cur.Specialties = append(cur.Specialties, x)
		}
	}
	if err := s.repo.UpsertProfile(ctx, cur); err != nil {
		return nil, err
	}
	return s.repo.Profile(ctx, u.ID)
}

// ---------- Dự án ----------

func (s *NTService) Projects(ctx context.Context, u *model.User, status string) ([]repository.NTProject, error) {
	if status != NTStatusRunning && status != NTStatusDone {
		status = ""
	}
	return s.repo.Projects(ctx, u.ID, status)
}

type NTProjectInput struct {
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	Kind     string   `json:"kind"`
	Status   string   `json:"status"`
	Progress *int     `json:"progress"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
}

func (s *NTService) CreateProject(ctx context.Context, u *model.User, in NTProjectInput) (*repository.NTProject, error) {
	p := &repository.NTProject{UserID: u.ID, Name: truncate(in.Name, 200), Address: truncate(in.Address, MaxAddressLen), Kind: truncate(in.Kind, 60)}
	if p.Name == "" {
		return nil, fail(ErrInvalidInput, "Cần tên dự án, ví dụ: Nhà phố Thủ Đức.")
	}
	if validLatLng(in.Lat, in.Lng) {
		p.Lat, p.Lng = in.Lat, in.Lng
	}
	if err := s.repo.CreateProject(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.Project(ctx, p.ID)
}

func validLatLng(lat, lng *float64) bool {
	return lat != nil && lng != nil && *lat > 7 && *lat < 24 && *lng > 101 && *lng < 118
}

func (s *NTService) UpdateProject(ctx context.Context, u *model.User, id int64, in NTProjectInput) (*repository.NTProject, error) {
	p, err := s.mine(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if n := truncate(in.Name, 200); n != "" {
		p.Name = n
	}
	p.Address = truncate(in.Address, MaxAddressLen)
	p.Kind = truncate(in.Kind, 60)
	if in.Progress != nil {
		p.Progress = min(max(*in.Progress, 0), 100)
	}
	switch in.Status {
	case NTStatusDone:
		if p.Status != NTStatusDone {
			p.FinishedOn = ptr(s.now())
		}
		p.Status, p.Progress = NTStatusDone, 100
	case NTStatusRunning:
		p.Status, p.FinishedOn = NTStatusRunning, nil
	}
	if validLatLng(in.Lat, in.Lng) {
		p.Lat, p.Lng = in.Lat, in.Lng
	}
	if err := s.repo.UpdateProject(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.Project(ctx, id)
}

func (s *NTService) DeleteProject(ctx context.Context, u *model.User, id int64) (string, error) {
	p, err := s.mine(ctx, u, id)
	if err != nil {
		return "", err
	}
	if err := s.repo.DeleteProject(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Đã xoá dự án %s.", p.Name), nil
}

type NTProjectDetail struct {
	Project *repository.NTProject `json:"project"`
	Logs    []repository.NTLog    `json:"logs"`
	Days    int                   `json:"days"` // ngày thi công thứ mấy (hoặc tổng số ngày nếu đã xong)
}

func (s *NTService) Project(ctx context.Context, u *model.User, id int64) (*NTProjectDetail, error) {
	p, err := s.mine(ctx, u, id)
	if err != nil {
		return nil, err
	}
	logs, err := s.repo.Logs(ctx, id)
	if err != nil {
		return nil, err
	}
	end := s.now()
	if p.FinishedOn != nil {
		end = *p.FinishedOn
	}
	for i := range logs {
		for j := range logs[i].Media {
			s.sign(&logs[i].Media[j])
		}
	}
	return &NTProjectDetail{Project: p, Logs: logs, Days: int(end.Sub(p.StartedOn).Hours()/24) + 1}, nil
}

// ---------- Nhật ký ----------

// NTPart: 1 phần của form tải nhật ký (ô chữ hoặc file), đọc lần lượt.
type NTPart struct {
	Field, Name, Mime string
	Body              io.Reader
}

type NTLogMeta struct {
	T    string   `json:"t"` // "YYYY-MM-DD HH:MM" giờ Việt Nam (in trên ảnh)
	Lat  *float64 `json:"lat"`
	Lng  *float64 `json:"lng"`
	Addr string   `json:"addr"`
}

type NTLogResult struct {
	Log     *repository.NTLog `json:"log"`
	Photos  int               `json:"photos"`
	Videos  int               `json:"videos"`
	Skipped []string          `json:"skipped"`
	Message string            `json:"message"`
}

// AddLog: 1 lần cập nhật nhật ký: ô "note", "meta" (JSON NTLogMeta) rồi các file "files" (ảnh JPG / PNG, video).
// next trả io.EOF khi hết phần.
func (s *NTService) AddLog(ctx context.Context, u *model.User, id int64, next func() (*NTPart, error)) (*NTLogResult, error) {
	p, err := s.mine(ctx, u, id)
	if err != nil {
		return nil, err
	}
	res := &NTLogResult{Skipped: []string{}}
	var log *repository.NTLog
	note := ""
	meta := NTLogMeta{}
	start := func() error {
		if log != nil {
			return nil
		}
		t := s.now()
		if tt, err := time.ParseInLocation("2006-01-02 15:04", meta.T, VN); err == nil && tt.Before(s.now().Add(10*time.Minute)) {
			t = tt
		}
		log = &repository.NTLog{ProjectID: p.ID, Note: note, TakenAt: t, Address: truncate(meta.Addr, MaxAddressLen)}
		if validLatLng(meta.Lat, meta.Lng) {
			log.Lat, log.Lng = meta.Lat, meta.Lng
		}
		return s.repo.CreateLog(ctx, log)
	}
	for {
		part, err := next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fail(ErrInvalidInput, "Không đọc được ảnh / video gửi lên.")
		}
		switch part.Field {
		case "note":
			b, _ := io.ReadAll(io.LimitReader(part.Body, 4000))
			note = truncate(string(b), 1000)
			continue
		case "meta":
			b, _ := io.ReadAll(io.LimitReader(part.Body, 4000))
			_ = json.Unmarshal(b, &meta)
			continue
		case "files":
		default:
			continue
		}
		if err := start(); err != nil {
			return nil, err
		}
		name := part.Name
		if mime := videoMime(name, part.Mime); mime != "" {
			sha, size, err := s.files.SaveVideo(part.Body, MaxVideoBytes)
			if errors.Is(err, storage.ErrTooBig) {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s: video quá %dMB", name, MaxVideoBytes>>20))
				continue
			}
			if err != nil {
				return nil, err
			}
			if err := s.repo.AddMedia(ctx, &repository.NTMedia{LogID: log.ID, ProjectID: p.ID, Kind: "video", SHA256: sha, Mime: mime, Size: size}); err != nil {
				if errors.Is(err, repository.ErrDuplicate) {
					res.Skipped = append(res.Skipped, name+": video đã có trong dự án")
					continue
				}
				return nil, err
			}
			res.Videos++
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part.Body, MaxFileBytes+1))
		if err != nil {
			return nil, fail(ErrInvalidInput, "Không đọc được ảnh gửi lên.")
		}
		if len(data) > MaxFileBytes {
			res.Skipped = append(res.Skipped, name+": ảnh quá 20MB")
			continue
		}
		if _, _, err := imaging.Probe(data); err != nil {
			res.Skipped = append(res.Skipped, name+": không phải ảnh")
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		if err := s.files.SavePhoto(sha, data, nil); err != nil {
			return nil, err
		}
		if err := s.repo.AddMedia(ctx, &repository.NTMedia{LogID: log.ID, ProjectID: p.ID, Kind: "photo", SHA256: sha, Mime: "image/jpeg", Size: int64(len(data))}); err != nil {
			if errors.Is(err, repository.ErrDuplicate) {
				res.Skipped = append(res.Skipped, name+": ảnh đã có trong dự án")
				continue
			}
			return nil, err
		}
		res.Photos++
	}
	if log == nil {
		return nil, fail(ErrInvalidInput, "Chụp ít nhất 1 ảnh hoặc video cho nhật ký hôm nay.")
	}
	if res.Photos+res.Videos == 0 {
		_ = s.repo.DeleteLog(ctx, log.ID)
		return nil, fail(ErrInvalidInput, "Không ảnh / video nào được nhận: %s", strings.Join(res.Skipped, "; "))
	}
	if log.Note != note { // ô note đến sau file (app khác thứ tự)
		_ = s.repo.SetLogNote(ctx, log.ID, note)
		log.Note = note
	}
	res.Log = log
	res.Message = fmt.Sprintf("Đã lưu nhật ký %s: %d ảnh%s.", p.Name, res.Photos, map[bool]string{true: fmt.Sprintf(", %d video", res.Videos), false: ""}[res.Videos > 0])
	return res, nil
}

func (s *NTService) DeleteLog(ctx context.Context, u *model.User, projectID, logID int64) (string, error) {
	if _, err := s.mine(ctx, u, projectID); err != nil {
		return "", err
	}
	logs, err := s.repo.Logs(ctx, projectID)
	if err != nil {
		return "", err
	}
	for _, l := range logs {
		if l.ID == logID {
			if err := s.repo.DeleteLog(ctx, logID); err != nil {
				return "", err
			}
			return "Đã xoá nhật ký.", nil
		}
	}
	return "", fail(ErrNotFound, "Không tìm thấy nhật ký.")
}

// ---------- Ảnh / video ----------

func (s *NTService) sig(id, exp int64) string {
	m := hmac.New(sha256.New, s.mediaKey)
	fmt.Fprintf(m, "%d.%d", id, exp)
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// sign: video xem qua link có chữ ký (trình phát video không gửi kèm token).
func (s *NTService) sign(m *repository.NTMedia) {
	if m.Kind != "video" {
		return
	}
	exp := s.now().Add(videoLinkTTL).Truncate(time.Hour).Add(time.Hour).Unix()
	m.Src = fmt.Sprintf("/api/v1/nt/videos/%d?e=%d&s=%s", m.ID, exp, s.sig(m.ID, exp))
}

// Photo: ảnh nhật ký (chủ dự án hoặc admin). small: ảnh thu nhỏ.
func (s *NTService) Photo(ctx context.Context, u *model.User, id int64, small bool) ([]byte, error) {
	m, err := s.repo.Media(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && m.Kind != "photo") {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.mine(ctx, u, m.ProjectID); err != nil {
		return nil, err
	}
	b, err := s.files.ReadPhoto(m.SHA256, small)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file ảnh.")
	}
	return b, nil
}

func (s *NTService) VideoFile(ctx context.Context, id int64, exp, sig string) (string, string, error) {
	e, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || e < s.now().Unix() || !hmac.Equal([]byte(sig), []byte(s.sig(id, e))) {
		return "", "", fail(ErrForbidden, "Link video đã hết hạn, tải lại trang.")
	}
	m, err := s.repo.Media(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && m.Kind != "video") {
		return "", "", fail(ErrNotFound, "Không tìm thấy video.")
	}
	if err != nil {
		return "", "", err
	}
	p, err := s.files.VideoPath(m.SHA256)
	return p, m.Mime, err
}

// ---------- Admin ----------

func (s *NTService) Contractors(ctx context.Context) ([]repository.NTContractor, error) {
	return s.repo.Contractors(ctx)
}

type NTAddInput struct {
	Phone   string `json:"phone"`
	Name    string `json:"name"`
	Company string `json:"company"`
}

// AddContractor: admin tạo tài khoản nhà thầu (đăng nhập app bằng SĐT).
func (s *NTService) AddContractor(ctx context.Context, by *model.User, in NTAddInput) (string, error) {
	phone, name := normPhone(in.Phone), truncate(in.Name, 100)
	if !ValidPhone(phone) || name == "" {
		return "", fail(ErrInvalidInput, "Cần SĐT đúng (10 số) và tên người liên hệ.")
	}
	if old, err := s.auth.FindByLogin(ctx, phone); err == nil {
		return "", fail(ErrConflict, "SĐT %s đã là tài khoản của %s.", phone, old.Name)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return "", err
	}
	hash, err := hashPassword(randomSecret())
	if err != nil {
		return "", err
	}
	u := &model.User{Username: phone, Name: name, Role: model.RoleNhaThau, PasswordHash: hash}
	if err := s.auth.CreateUser(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return "", fail(ErrConflict, "SĐT %s đã có tài khoản.", phone)
		}
		return "", err
	}
	if err := s.repo.UpsertProfile(ctx, &repository.NTProfile{UserID: u.ID, Company: truncate(in.Company, 200)}); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActAddNT, nil, nil, fmt.Sprintf("%s (%s) %s", name, phone, in.Company))
	return fmt.Sprintf("Đã thêm nhà thầu %s. Mở app Vua Thầu Map, nhập SĐT %s là vào.", name, phone), nil
}
