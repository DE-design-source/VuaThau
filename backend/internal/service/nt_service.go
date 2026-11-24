package service

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/province"
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
	geo      NTGeocoder
	sites    NTSites
	mediaKey []byte
	now      func() time.Time
	views    viewSeen // lượt xem hồ sơ năng lực: mỗi người xem tính 1 lần / 6 giờ
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
	Sites    []NTSite               `json:"sites"` // công trình Vua Thầu ghi nhận (CTV chụp bảng hiệu có SĐT nhà thầu)
	Stats    struct {
		Sites    int `json:"sites"`
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
	sites, err := s.Sites(ctx, u)
	if err != nil {
		return nil, err
	}
	s.covers(ps)
	h := &NTHome{User: u, Profile: prof, Projects: ps, Sites: sites}
	h.Stats.Sites = len(sites)
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
	ps, err := s.repo.Projects(ctx, u.ID, status)
	if err == nil {
		s.covers(ps)
	}
	return ps, err
}

type NTProjectInput struct {
	Name     string `json:"name"`
	Address  string `json:"address"`  // địa chỉ 1 dòng (app cũ); có province thì bỏ qua, ghép từ 3 ô dưới
	Province string `json:"province"` // form Đăng dự án: tỉnh / thành
	Ward     string `json:"ward"`     // phường / xã / đặc khu (đúng danh sách 2025)
	Street   string `json:"street"`   // số nhà, tên đường
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	// FinishedYear: form Đăng dự án, Tình trạng "Đã hoàn thành" -> Năm hoàn thành
	FinishedYear *int     `json:"finished_year"`
	Progress     *int     `json:"progress"`
	Lat          *float64 `json:"lat"`
	Lng          *float64 `json:"lng"`
}

// NTGeocoder: tra toạ độ từ địa chỉ dự án khi nhà thầu không ghim trên bản đồ.
type NTGeocoder interface {
	Search(ctx context.Context, q string) ([]geo.Place, error)
}

// NTSites: công trình CTV đã chụp theo SĐT nhà thầu (houseService.ContractorSites) + link công khai ảnh hồ sơ CTV.
type NTSites interface {
	ContractorSites(ctx context.Context, phone string) ([]NTSite, error)
	PublicPhotoURL(pid int64) string
}

// covers: dự án tạo từ công trình CTV mà chưa có ảnh của nhà thầu: ảnh bìa là ảnh đầu của hồ sơ CTV (link công khai).
func (s *NTService) covers(ps []repository.NTProject) {
	if s.sites == nil {
		return
	}
	for i := range ps {
		if ps[i].Cover == nil && ps[i].HousePhoto != nil {
			ps[i].CoverURL = s.sites.PublicPhotoURL(*ps[i].HousePhoto)
		}
	}
}

// ClaimSite: nhà thầu chụp nhật ký vào công trình CTV đã chụp có SĐT của mình: tạo (1 lần) dự án gắn với hồ sơ đó,
// địa chỉ / tỉnh / toạ độ lấy từ hồ sơ. Đã có thì trả dự án cũ. Công trình không có SĐT nhà thầu này: 404.
func (s *NTService) ClaimSite(ctx context.Context, u *model.User, hid int64) (*repository.NTProject, error) {
	if p, err := s.repo.ProjectByHouse(ctx, u.ID, hid); err == nil {
		return p, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	all, err := s.allSites(ctx, u)
	if err != nil {
		return nil, err
	}
	var site *NTSite
	for i := range all {
		if all[i].ID == hid {
			site = &all[i]
		}
	}
	if site == nil {
		return nil, fail(ErrNotFound, "Không tìm thấy công trình có số điện thoại của bạn.")
	}
	name := strings.TrimSpace(strings.Split(site.Address, ",")[0])
	if name == "" {
		name = fmt.Sprintf("Công trình HS-%05d", site.ID)
	}
	hidCopy := hid
	p := &repository.NTProject{UserID: u.ID, Name: truncate(name, 200), Address: truncate(site.Address, MaxAddressLen),
		Province: site.ProvinceName, HouseID: &hidCopy}
	if validLatLng(site.Lat, site.Lng) {
		p.Lat, p.Lng = site.Lat, site.Lng
	}
	if err := s.repo.CreateProject(ctx, p); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return s.repo.ProjectByHouse(ctx, u.ID, hid)
		}
		return nil, err
	}
	out, err := s.repo.Project(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	one := []repository.NTProject{*out}
	s.covers(one)
	return &one[0], nil
}

// WithSites: Trang chủ / Dự án của nhà thầu có thêm công trình Vua Thầu ghi nhận theo SĐT (nil: không có).
func (s *NTService) WithSites(src NTSites) *NTService {
	s.sites = src
	return s
}

func (s *NTService) allSites(ctx context.Context, u *model.User) ([]NTSite, error) {
	if s.sites == nil {
		return []NTSite{}, nil
	}
	alt, err := s.repo.AltPhones(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	out := []NTSite{}
	seen := map[int64]bool{}
	for _, phone := range append([]string{u.Username}, alt...) {
		rows, err := s.sites.ContractorSites(ctx, phone)
		if err != nil {
			return nil, err
		}
		for _, x := range rows {
			if !seen[x.ID] {
				seen[x.ID] = true
				out = append(out, x)
			}
		}
	}
	return out, nil
}

// maxAltPhones: số SĐT phụ tối đa mỗi nhà thầu.
const maxAltPhones = 5

// SetAltPhones (admin): gán SĐT phụ cho nhà thầu uid. Số phụ không được là số đăng nhập của tài khoản khác, cũng
// không được là số phụ của nhà thầu khác (1 công trình chỉ hiện cho 1 nhà thầu).
func (s *NTService) SetAltPhones(ctx context.Context, by *model.User, uid int64, phones []string) ([]string, error) {
	phone, err := s.repo.UserPhone(ctx, uid) // chỉ tài khoản nhà thầu
	if err != nil {
		return nil, err
	}
	if phone == "" {
		return nil, fail(ErrNotFound, "Không tìm thấy nhà thầu.")
	}
	u, err := s.auth.FindByLogin(ctx, phone)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, raw := range phones {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		p := normPhone(raw)
		if !ValidPhone(p) {
			return nil, fail(ErrInvalidInput, "SĐT phụ %q không đúng (cần 10 số).", raw)
		}
		if p == u.Username || slices.Contains(out, p) {
			continue
		}
		if other, err := s.auth.FindByLogin(ctx, p); err == nil {
			return nil, fail(ErrConflict, "SĐT %s là số đăng nhập của %s.", p, other.Name)
		} else if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if owner, err := s.repo.AltPhoneOwner(ctx, p); err != nil {
			return nil, err
		} else if owner != 0 && owner != uid {
			return nil, fail(ErrConflict, "SĐT %s đã là số phụ của nhà thầu khác.", p)
		}
		out = append(out, p)
	}
	if len(out) > maxAltPhones {
		return nil, fail(ErrInvalidInput, "Tối đa %d SĐT phụ.", maxAltPhones)
	}
	if err := s.repo.SetAltPhones(ctx, uid, out); err != nil {
		return nil, err
	}
	_ = s.logs.Add(ctx, by.ID, ActNTAltPhones, nil, nil, fmt.Sprintf("%s (%s): %s", u.Name, u.Username, strings.Join(out, ", ")))
	return out, nil
}

// Sites: công trình CTV đã chụp có bảng hiệu ghi SĐT của nhà thầu này, trừ công trình đã thành dự án của nhà thầu
// (đã chụp nhật ký vào: hiện ở danh sách dự án, không lặp).
func (s *NTService) Sites(ctx context.Context, u *model.User) ([]NTSite, error) {
	all, err := s.allSites(ctx, u)
	if err != nil || len(all) == 0 {
		return all, err
	}
	ps, err := s.repo.Projects(ctx, u.ID, "")
	if err != nil {
		return nil, err
	}
	claimed := map[int64]bool{}
	for _, p := range ps {
		if p.HouseID != nil {
			claimed[*p.HouseID] = true
		}
	}
	out := []NTSite{}
	for _, x := range all {
		if !claimed[x.ID] {
			out = append(out, x)
		}
	}
	return out, nil
}

// WithGeo: bật tra toạ độ theo địa chỉ (nil: tắt).
func (s *NTService) WithGeo(g NTGeocoder) *NTService {
	s.geo = g
	return s
}

// address: điền địa chỉ dự án theo form (tỉnh + phường bắt buộc, đúng danh sách sau sắp xếp 2025).
func (s *NTService) address(p *repository.NTProject, in NTProjectInput) error {
	if strings.TrimSpace(in.Province) == "" && strings.TrimSpace(in.Ward) == "" {
		p.Address = truncate(in.Address, MaxAddressLen) // app cũ: 1 ô địa chỉ
		return nil
	}
	prov, ward, ok := province.Ward(in.Province, in.Ward)
	if prov == "" {
		return fail(ErrInvalidInput, "Chọn tỉnh / thành phố.")
	}
	if !ok {
		return fail(ErrInvalidInput, "Chọn phường / xã thuộc %s.", prov)
	}
	p.Province, p.Ward, p.Street = prov, ward, truncate(in.Street, 200)
	p.Address = truncate(province.Join(p.Street, p.Ward, p.Province), MaxAddressLen)
	return nil
}

// locate: chưa ghim trên bản đồ thì tra toạ độ theo địa chỉ (lỗi / không ra: để trống, nhật ký đầu tiên có GPS sẽ điền).
func (s *NTService) locate(ctx context.Context, p *repository.NTProject) {
	if s.geo == nil || p.Lat != nil || strings.TrimSpace(p.Address) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	places, err := s.geo.Search(ctx, p.Address)
	if err != nil {
		return
	}
	// Tra địa chỉ hay ra chỗ khác hẳn (vd "20 Nguyễn Thiện Thành, P. Tân Khánh" ra một chùa ở Cát Lái):
	// chỉ nhận điểm trong đúng tỉnh đã chọn VÀ tên có phường hoặc tên đường đã nhập; không có thì để trống,
	// nhật ký đầu tiên có GPS sẽ điền (ghim sai còn tệ hơn chưa có ghim).
	wardKey := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(province.Key(p.Ward), "phuong "), "xa "), "dac khu "))
	streetKey := strings.TrimSpace(strings.TrimLeft(province.Key(p.Street), "0123456789 "))
	for _, pl := range places {
		lat, lng := pl.Lat, pl.Lng
		if !validLatLng(&lat, &lng) {
			continue
		}
		if p.Province != "" {
			if at := province.At(lat, lng); at == nil || !province.SameProvince(at.Name, p.Province) {
				continue
			}
			name := " " + province.Key(pl.Name) + " "
			if !(wardKey != "" && strings.Contains(name, " "+wardKey+" ")) && !(streetKey != "" && strings.Contains(name, " "+streetKey+" ")) {
				continue
			}
		}
		p.Lat, p.Lng = &lat, &lng
		return
	}
}

func (s *NTService) CreateProject(ctx context.Context, u *model.User, in NTProjectInput) (*repository.NTProject, error) {
	p := &repository.NTProject{UserID: u.ID, Name: truncate(in.Name, 200), Kind: truncate(in.Kind, 60)}
	if p.Name == "" {
		return nil, fail(ErrInvalidInput, "Cần tên dự án, ví dụ: Nhà phố Thủ Đức.")
	}
	if err := s.address(p, in); err != nil {
		return nil, err
	}
	if validLatLng(in.Lat, in.Lng) {
		p.Lat, p.Lng = in.Lat, in.Lng
	}
	s.locate(ctx, p)
	if err := s.repo.CreateProject(ctx, p); err != nil {
		return nil, err
	}
	// form Đăng dự án: Tình trạng "Đã hoàn thành" + Năm hoàn thành (dự án cũ đưa vào hồ sơ năng lực)
	if in.Status == NTStatusDone {
		p.Status, p.Progress, p.FinishedOn = NTStatusDone, 100, s.finishedOn(in.FinishedYear)
		if err := s.repo.UpdateProject(ctx, p); err != nil {
			return nil, err
		}
	}
	return s.repo.Project(ctx, p.ID)
}

// finishedOn: ngày hoàn thành theo năm chọn ở form (năm nay / không chọn: hôm nay; năm trước: 31/12 năm đó).
func (s *NTService) finishedOn(year *int) *time.Time {
	now := s.now()
	if year == nil || *year >= now.Year() || *year < 1990 {
		return &now
	}
	t := time.Date(*year, 12, 31, 12, 0, 0, 0, VN)
	return &t
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
	oldAddr := p.Address
	if err := s.address(p, in); err != nil {
		return nil, err
	}
	p.Kind = truncate(in.Kind, 60)
	if in.Progress != nil {
		p.Progress = min(max(*in.Progress, 0), 100)
	}
	switch in.Status {
	case NTStatusDone:
		if p.Status != NTStatusDone || in.FinishedYear != nil {
			p.FinishedOn = s.finishedOn(in.FinishedYear)
		}
		p.Status, p.Progress = NTStatusDone, 100
	case NTStatusRunning:
		p.Status, p.FinishedOn = NTStatusRunning, nil
	}
	if validLatLng(in.Lat, in.Lng) {
		p.Lat, p.Lng = in.Lat, in.Lng
	} else if p.Address != oldAddr {
		p.Lat, p.Lng = nil, nil // đổi địa chỉ mà không ghim lại: tra lại toạ độ
		s.locate(ctx, p)
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
	Photos  []repository.NTMedia  `json:"photos"` // ảnh dự án (form Đăng dự án), ảnh bìa trước
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
	photos, err := s.repo.ProjectPhotos(ctx, id)
	if err != nil {
		return nil, err
	}
	one := []repository.NTProject{*p}
	s.covers(one)
	// dự án cũ đưa vào hồ sơ (hoàn thành năm trước, ngày bắt đầu = ngày đăng): không ra số ngày âm
	return &NTProjectDetail{Project: &one[0], Photos: photos, Logs: logs, Days: max(1, int(end.Sub(p.StartedOn).Hours()/24)+1)}, nil
}

// MaxNTProjectPhotos: ảnh dự án tối đa (thiết kế: "Ảnh bìa đặt ở ô đầu tiên · tối đa 20 ảnh").
const MaxNTProjectPhotos = 20

// AddProjectPhotos: ảnh dự án (form Đăng dự án), các phần "files" theo thứ tự (ảnh đầu = ảnh bìa nếu dự án chưa có ảnh).
func (s *NTService) AddProjectPhotos(ctx context.Context, u *model.User, id int64, next func() (*NTPart, error)) (*NTLogResult, error) {
	p, err := s.mine(ctx, u, id)
	if err != nil {
		return nil, err
	}
	have, err := s.repo.ProjectPhotos(ctx, id)
	if err != nil {
		return nil, err
	}
	res := &NTLogResult{Skipped: []string{}}
	for {
		part, err := next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fail(ErrInvalidInput, "Không đọc được ảnh gửi lên.")
		}
		if part.Field != "files" {
			continue
		}
		if len(have)+res.Photos >= MaxNTProjectPhotos {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: dự án đã đủ %d ảnh", part.Name, MaxNTProjectPhotos))
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part.Body, MaxFileBytes+1))
		if err != nil {
			return nil, fail(ErrInvalidInput, "Không đọc được ảnh gửi lên.")
		}
		if len(data) > MaxFileBytes {
			res.Skipped = append(res.Skipped, part.Name+": ảnh quá 20MB")
			continue
		}
		if _, _, err := imaging.Probe(data); err != nil {
			res.Skipped = append(res.Skipped, part.Name+": "+probeWhy(err))
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		if err := s.files.SavePhoto(sha, data, nil); err != nil {
			return nil, err
		}
		if err := s.repo.AddMedia(ctx, &repository.NTMedia{ProjectID: p.ID, Kind: "photo", SHA256: sha, Mime: "image/jpeg", Size: int64(len(data))}); err != nil {
			if errors.Is(err, repository.ErrDuplicate) {
				res.Skipped = append(res.Skipped, part.Name+": ảnh đã có trong dự án")
				continue
			}
			return nil, err
		}
		res.Photos++
	}
	if res.Photos == 0 && len(res.Skipped) > 0 {
		return nil, fail(ErrInvalidInput, "Không ảnh nào được nhận: %s", strings.Join(res.Skipped, "; "))
	}
	res.Message = fmt.Sprintf("Đã thêm %d ảnh cho dự án %s.", res.Photos, p.Name)
	return res, nil
}

// ---------- Báo cáo gửi chủ nhà ----------
// Mỗi nhật ký có 1 link công khai /bc/<mã> (mã = id nhật ký + chữ ký, không hết hạn): chủ nhà mở xem ảnh thực tế,
// không cần đăng nhập. Không lưu gì thêm; xoá nhật ký / dự án thì link hết hiệu lực.

func (s *NTService) reportSig(logID int64) string {
	m := hmac.New(sha256.New, s.mediaKey)
	fmt.Fprintf(m, "bc.%d", logID)
	return hex.EncodeToString(m.Sum(nil))[:12]
}

// ReportCode: mã báo cáo của nhật ký, dạng "<id>-<chữ ký>".
func (s *NTService) ReportCode(logID int64) string {
	return fmt.Sprintf("%d-%s", logID, s.reportSig(logID))
}

func (s *NTService) reportLog(code string) (int64, bool) {
	id, sig, ok := strings.Cut(code, "-")
	n, err := strconv.ParseInt(id, 10, 64)
	return n, ok && err == nil && n > 0 && hmac.Equal([]byte(sig), []byte(s.reportSig(n)))
}

type NTReportLink struct {
	Code string `json:"code"`
	Path string `json:"path"` // /bc/<mã>: app ghép với tên miền đang dùng
}

// Report: link báo cáo của 1 nhật ký (chủ dự án).
func (s *NTService) Report(ctx context.Context, u *model.User, projectID, logID int64) (*NTReportLink, error) {
	if _, err := s.mine(ctx, u, projectID); err != nil {
		return nil, err
	}
	l, err := s.repo.Log(ctx, logID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && l.ProjectID != projectID) {
		return nil, fail(ErrNotFound, "Không tìm thấy nhật ký.")
	}
	if err != nil {
		return nil, err
	}
	c := s.ReportCode(l.ID)
	return &NTReportLink{Code: c, Path: "/bc/" + c}, nil
}

// NTPublicReport: trang báo cáo công khai (chủ nhà xem). Không có SĐT / tài khoản nhà thầu.
type NTPublicReport struct {
	Company  string `json:"company"`
	Regions  string `json:"regions"` // khu vực hoạt động (đầu trang báo cáo)
	Verified bool   `json:"verified"`
	Phone    string `json:"phone"` // SĐT nhà thầu: nút Gọi nhà thầu / Nhắn Zalo (nhà thầu tự gửi link nên chủ nhà được thấy)
	// Portfolio: hồ sơ năng lực của nhà thầu (/nt/<mã>): nút "<" khi mở thẳng link báo cáo (không có trang trước)
	Portfolio string        `json:"portfolio"`
	Project   string        `json:"project"`
	Address   string        `json:"address"`
	Kind      string        `json:"kind"`
	Status    string        `json:"status"`
	Progress  int           `json:"progress"`
	Days      int           `json:"days"`  // ngày thi công thứ mấy
	Logs      int           `json:"logs"`  // số nhật ký của dự án
	Total     int           `json:"total"` // số ảnh của dự án
	Note      string        `json:"note"`
	TakenAt   time.Time     `json:"taken_at"`
	GPS       bool          `json:"gps"` // ảnh có toạ độ GPS + giờ chụp in trên ảnh
	Lat       *float64      `json:"lat"`
	Lng       *float64      `json:"lng"`
	LogAddr   string        `json:"log_address"` // địa chỉ in trên ảnh (màn xem ảnh)
	Photos    []string      `json:"photos"`
	Videos    int           `json:"videos"`
	Earlier   []NTPublicLog `json:"earlier"` // nhật ký trước đây của dự án (mới trước)
}

// NTPublicLog: 1 nhật ký trước đây trên trang báo cáo.
type NTPublicLog struct {
	TakenAt time.Time `json:"taken_at"`
	Note    string    `json:"note"`
	GPS     bool      `json:"gps"`
	Address string    `json:"address"`
	Photos  []string  `json:"photos"`
}

// maxEarlierLogs: trang báo cáo hiện tối đa bấy nhiêu nhật ký trước đây.
const maxEarlierLogs = 30

func (s *NTService) PublicReport(ctx context.Context, code string) (*NTPublicReport, error) {
	id, ok := s.reportLog(code)
	if !ok {
		return nil, fail(ErrNotFound, "Link báo cáo không đúng.")
	}
	l, err := s.repo.Log(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrNotFound, "Báo cáo này không còn.")
	}
	if err != nil {
		return nil, err
	}
	p, err := s.repo.Project(ctx, l.ProjectID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrNotFound, "Báo cáo này không còn.")
	}
	if err != nil {
		return nil, err
	}
	// nhà thầu bị khoá hoặc đã xoá: link báo cáo thôi hiện
	phone, err := s.repo.PublicPhone(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	if phone == "" {
		return nil, fail(ErrNotFound, "Báo cáo này không còn.")
	}
	prof, err := s.repo.Profile(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	end := s.now()
	if p.FinishedOn != nil {
		end = *p.FinishedOn
	}
	url := func(mid int64) string { return fmt.Sprintf("/api/v1/public/bc/%s/photos/%d", code, mid) }
	r := &NTPublicReport{Company: prof.Company, Regions: prof.Regions, Verified: prof.Verified, Phone: phone, Project: p.Name,
		Portfolio: fmt.Sprintf("/nt/%d-%s", p.UserID, s.portfolioSig(p.UserID)),
		Address:   p.Address, Kind: p.Kind, Status: p.Status, Progress: p.Progress, Days: max(1, int(end.Sub(p.StartedOn).Hours()/24)+1),
		Logs: p.Logs, Total: p.Photos, Note: l.Note, TakenAt: l.TakenAt, GPS: l.Lat != nil, Lat: l.Lat, Lng: l.Lng, LogAddr: l.Address,
		Photos: []string{}, Earlier: []NTPublicLog{}}
	for _, m := range l.Media {
		if m.Kind == "photo" {
			r.Photos = append(r.Photos, url(m.ID))
		} else {
			r.Videos++
		}
	}
	logs, err := s.repo.Logs(ctx, p.ID) // mới trước
	if err != nil {
		return nil, err
	}
	for _, x := range logs {
		if x.ID == l.ID || len(r.Earlier) >= maxEarlierLogs || x.TakenAt.After(l.TakenAt) {
			continue // chỉ nhật ký TRƯỚC lần báo cáo này
		}
		e := NTPublicLog{TakenAt: x.TakenAt, Note: x.Note, GPS: x.Lat != nil, Address: x.Address, Photos: []string{}}
		for _, m := range x.Media {
			if m.Kind == "photo" {
				e.Photos = append(e.Photos, url(m.ID))
			}
		}
		if len(e.Photos) > 0 {
			r.Earlier = append(r.Earlier, e)
		}
	}
	return r, nil
}

// PublicReportPhoto: ảnh của báo cáo: ảnh nhật ký trong mã, hoặc nhật ký TRƯỚC đó của cùng dự án ("Nhật ký trước đây").
// Ảnh dự án (không thuộc nhật ký) và nhật ký sau lần báo cáo: không.
func (s *NTService) PublicReportPhoto(ctx context.Context, code string, mediaID int64, small bool) ([]byte, error) {
	id, ok := s.reportLog(code)
	if !ok {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	m, err := s.repo.Media(ctx, mediaID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && (m.Kind != "photo" || m.LogID == 0)) {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if err != nil {
		return nil, err
	}
	base, err := s.repo.Log(ctx, id)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if bp, err := s.repo.Project(ctx, base.ProjectID); err != nil || !s.publicOpen(ctx, bp.UserID) {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if m.LogID != id {
		other, err := s.repo.Log(ctx, m.LogID)
		if err != nil || other.ProjectID != base.ProjectID || other.TakenAt.After(base.TakenAt) {
			return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
		}
	}
	b, err := s.files.ReadPhoto(m.SHA256, small)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file ảnh.")
	}
	return b, nil
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
//
// Gửi lại sau khi mất mạng: lần trước dở dang thì nhật ký dở bị xoá (lỗi giữa chừng không để lại nhật ký thiếu ảnh /
// nhật ký rỗng); lần trước đã lưu xong mà app không nhận được trả lời thì mọi file đều trùng 1 nhật ký đã có: trả lại
// nhật ký đó như vừa lưu (trước đây báo lỗi "đã có trong dự án", nhà thầu kẹt không tới được màn báo cáo).
func (s *NTService) AddLog(ctx context.Context, u *model.User, id int64, next func() (*NTPart, error)) (res *NTLogResult, err error) {
	p, err := s.mine(ctx, u, id)
	if err != nil {
		return nil, err
	}
	res = &NTLogResult{Skipped: []string{}}
	var log *repository.NTLog
	defer func() {
		if err != nil && log != nil {
			_ = s.repo.DeleteLog(context.WithoutCancel(ctx), log.ID)
		}
	}()
	note := ""
	meta := NTLogMeta{}
	files := 0
	dupLog, dupOnly := int64(0), true // các file trùng thuộc nhật ký nào (cùng 1 nhật ký: lần gửi trước đã lưu xong)
	dupOf := func(sha string) {
		if lid, e := s.repo.LogOfMedia(ctx, p.ID, sha); e != nil || lid == 0 || (dupLog != 0 && lid != dupLog) {
			dupOnly = false
		} else {
			dupLog = lid
		}
	}
	start := func() error {
		if log != nil {
			return nil
		}
		t := s.now()
		// giờ chụp app gửi: không ở tương lai, không lùi quá MaxLogAge (ảnh chụp lúc mất mạng rồi gửi sau vẫn nhận)
		if tt, err := time.ParseInLocation("2006-01-02 15:04", meta.T, VN); err == nil && tt.Before(s.now().Add(10*time.Minute)) && tt.After(s.now().Add(-MaxLogAge)) {
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
			// chữ sai bảng mã (máy gửi Windows-1252...) làm Postgres báo lỗi: bỏ ký tự hỏng thay vì lỗi 500
			note = truncate(strings.ToValidUTF8(string(b), ""), 1000)
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
		if files++; files > MaxLogFiles {
			_, _ = io.Copy(io.Discard, part.Body)
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: mỗi nhật ký tối đa %d ảnh / video", name, MaxLogFiles))
			dupOnly = false
			continue
		}
		if mime := videoMime(name, part.Mime); mime != "" {
			br := bufio.NewReaderSize(part.Body, 64)
			if head, _ := br.Peek(12); !looksLikeVideo(head) {
				_, _ = io.Copy(io.Discard, br)
				res.Skipped = append(res.Skipped, name+": không phải file video")
				dupOnly = false
				continue
			}
			sha, size, err := s.files.SaveVideo(br, MaxVideoBytes)
			if errors.Is(err, storage.ErrTooBig) {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s: video quá %dMB", name, MaxVideoBytes>>20))
				dupOnly = false
				continue
			}
			if err != nil {
				return nil, fail(ErrInvalidInput, "Không đọc được video gửi lên, thử lại.")
			}
			if err := s.repo.AddMedia(ctx, &repository.NTMedia{LogID: log.ID, ProjectID: p.ID, Kind: "video", SHA256: sha, Mime: mime, Size: size}); err != nil {
				if errors.Is(err, repository.ErrDuplicate) {
					res.Skipped = append(res.Skipped, name+": video đã có trong dự án")
					dupOf(sha)
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
			dupOnly = false
			continue
		}
		if _, _, err := imaging.Probe(data); err != nil {
			res.Skipped = append(res.Skipped, name+": "+probeWhy(err))
			dupOnly = false
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
				dupOf(sha)
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
		log = nil
		// gửi lại đúng nhật ký đã lưu xong (app không nhận được trả lời lần trước): trả nhật ký đó
		if dupOnly && dupLog != 0 {
			if old, e := s.repo.Log(ctx, dupLog); e == nil && old.ProjectID == p.ID {
				for _, m := range old.Media {
					if m.Kind == "photo" {
						res.Photos++
					} else {
						res.Videos++
					}
				}
				res.Log, res.Skipped = old, []string{}
				res.Message = fmt.Sprintf("Nhật ký %s đã được lưu từ lần gửi trước.", p.Name)
				return res, nil
			}
		}
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

// EnsureFromLark: nhà thầu đăng ký qua form Lark và đã tích "Xác minh tham gia" -> tự có tài khoản app (đăng nhập bằng SĐT).
// SĐT đã có tài khoản nhà thầu: chỉ bổ sung hồ sơ còn trống (không ghi đè những gì nhà thầu đã tự sửa trong app).
// SĐT là tài khoản CTV / nhân viên: bỏ qua (1 SĐT 1 tài khoản). Trả true khi vừa tạo tài khoản mới.
func (s *NTService) EnsureFromLark(ctx context.Context, c *repository.RegContractor) (bool, error) {
	phone := normPhone(c.Phone)
	if !ValidPhone(phone) {
		return false, nil
	}
	company := truncate(c.Name, 200)
	person := truncate(c.Contact, 100)
	if person == "" {
		person = truncate(c.Name, 100)
	}
	if person == "" {
		return false, nil
	}
	u, err := s.auth.FindByLogin(ctx, phone)
	if err == nil {
		if u.Role != model.RoleNhaThau {
			return false, nil
		}
		// gắn dòng Lark (để lần sau bỏ tích thì khoá được); bị khoá vì bỏ tích trước đó mà nay tích lại: mở lại
		if c.LarkRecord != "" {
			if err := s.repo.SetLarkRecord(ctx, u.ID, c.LarkRecord); err != nil {
				return false, err
			}
		}
		if u.Locked == model.LockedUnverified {
			if ok, err := s.repo.SwapLock(ctx, u.ID, model.LockedUnverified, model.LockedNone); err != nil {
				return false, err
			} else if ok {
				_ = s.logs.Add(ctx, 0, ActUnlock, nil, nil, fmt.Sprintf("Nhà thầu %s (%s): tích lại Xác minh tham gia trên form Lark", u.Name, phone))
			}
		}
		prof, err := s.repo.Profile(ctx, u.ID)
		if err != nil {
			return false, err
		}
		changed := false
		if prof.Company == "" && company != "" {
			prof.Company, changed = company, true
		}
		if prof.Regions == "" && c.Areas != "" {
			prof.Regions, changed = truncate(c.Areas, 200), true
		}
		if !prof.Verified {
			prof.Verified, changed = true, true
		}
		if changed {
			return false, s.repo.UpsertProfile(ctx, prof)
		}
		return false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return false, err
	}
	// nhà thầu đã tự xoá tài khoản: không tạo lại từ form Lark
	if gone, err := s.auth.PhoneDeleted(ctx, phone); err != nil || gone {
		return false, err
	}
	hash, err := hashPassword(randomSecret())
	if err != nil {
		return false, err
	}
	nu := &model.User{Username: phone, Name: person, Role: model.RoleNhaThau, PasswordHash: hash}
	if err := s.auth.CreateUser(ctx, nu); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return false, nil // lượt đồng bộ khác vừa tạo
		}
		return false, err
	}
	if err := s.repo.UpsertProfile(ctx, &repository.NTProfile{UserID: nu.ID, Company: company, Regions: truncate(c.Areas, 200), Verified: true}); err != nil {
		return false, err
	}
	if err := s.repo.SetLarkRecord(ctx, nu.ID, c.LarkRecord); err != nil {
		return false, err
	}
	_ = s.logs.Add(ctx, 0, ActAddNT, nil, nil, fmt.Sprintf("%s (%s) %s · tự tạo từ form Lark đã xác minh", person, phone, company))
	return true, nil
}

// RevokeFromLark: SĐT trên form Lark không còn dòng nào được tích "Xác minh tham gia" -> khoá tài khoản nhà thầu
// (chỉ tài khoản gắn với form Lark; tài khoản admin tạo tay mà không có trên form thì không đụng). Đăng xuất mọi máy.
// Khoá kiểu LockedUnverified: tích lại thì EnsureFromLark tự mở; admin khoá tay (LockedByAdmin) không bị mở. Trả true khi vừa khoá.
func (s *NTService) RevokeFromLark(ctx context.Context, phone string) (bool, error) {
	phone = normPhone(phone)
	if !ValidPhone(phone) {
		return false, nil
	}
	u, err := s.auth.FindByLogin(ctx, phone)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if u.Role != model.RoleNhaThau || u.Locked != model.LockedNone {
		return false, nil
	}
	rec, err := s.repo.LarkRecord(ctx, u.ID)
	// chưa gắn form, hoặc nhà thầu tự đăng ký trong app đã được admin xác minh ("app-..."): không khoá theo Lark
	if err != nil || rec == "" || strings.HasPrefix(rec, appRecordPrefix) {
		return false, err
	}
	ok, err := s.repo.SwapLock(ctx, u.ID, model.LockedNone, model.LockedUnverified)
	if err != nil || !ok {
		return false, err
	}
	if prof, err := s.repo.Profile(ctx, u.ID); err == nil && prof.Verified {
		prof.Verified = false
		_ = s.repo.UpsertProfile(ctx, prof)
	}
	_ = s.logs.Add(ctx, 0, ActLock, nil, nil, fmt.Sprintf("Nhà thầu %s (%s): bỏ tích Xác minh tham gia trên form Lark", u.Name, phone))
	return true, nil
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

const (
	// MaxLogFiles: số ảnh + video tối đa của 1 lần cập nhật nhật ký
	MaxLogFiles = 60
	// MaxLogAge: giờ chụp app gửi lùi quá chừng này thì dùng giờ máy chủ (không ghi lùi nhật ký tuỳ ý)
	MaxLogAge = 30 * 24 * time.Hour
)

// publicOpen: tài khoản nhà thầu còn mở (trang công khai /nt, /bc và ảnh của chúng chỉ hiện khi còn mở).
func (s *NTService) publicOpen(ctx context.Context, uid int64) bool {
	phone, err := s.repo.PublicPhone(ctx, uid)
	return err == nil && phone != ""
}

// probeWhy: lý do bỏ 1 file ảnh nhà thầu gửi (không phải ảnh / ảnh quá lớn).
func probeWhy(err error) string {
	if errors.Is(err, imaging.ErrTooBig) {
		return "ảnh quá lớn, chụp lại ở độ phân giải thấp hơn"
	}
	return "không phải ảnh"
}
