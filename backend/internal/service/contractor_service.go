package service

import (
	"context"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/integration/lark"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// Nhà thầu (góp ý 23/11/2026, trang admin /contractors): 2 nguồn gộp lại, khớp theo SĐT
//   - đăng ký qua Lark Base "Form Đăng ký nhà thầu - Vua Thầu" (đồng bộ mỗi 15 phút vào bảng contractors);
//   - trích từ ảnh CTV chụp công trình (tên / SĐT nhà thầu trên bảng hiệu, houses.contractor_*).
// Thêm nhà thầu: nút "Thêm nhà thầu" mở form Lark, dòng mới tự về sau lượt đồng bộ kế tiếp.

// ContractorRow: 1 dòng trang Nhà thầu.
type ContractorRow struct {
	Key        string    `json:"key"` // SĐT (chỉ số) / "ten:<tên>" (ảnh CTV không có SĐT) / "nt:<id>" (đăng ký không có SĐT)
	Name       string    `json:"name"`
	Kind       string    `json:"kind"` // Công ty / Tổ đội / Cá nhân (đăng ký); trống: chưa rõ
	Role       string    `json:"role"`
	Phone      string    `json:"phone"`
	Areas      string    `json:"areas"`
	Exp        string    `json:"exp"`
	ValueRange string    `json:"value_range"`
	Total      int       `json:"total"` // số công trình CTV đã chụp
	Status     string    `json:"status"`
	Registered bool      `json:"registered"` // có đăng ký qua form
	Photo      bool      `json:"photo"`      // có công trình từ ảnh CTV
	Joined     time.Time `json:"joined"`
	Updated    time.Time `json:"updated"`
}

// ContractorDetail: trang chi tiết 1 nhà thầu.
type ContractorDetail struct {
	ContractorRow
	Profile  *repository.RegContractor `json:"profile"` // nil: chỉ có từ ảnh CTV
	Projects []repository.SaleProject  `json:"projects"`
}

const (
	NTVerified   = "Đã xác minh"
	NTUnverified = "Chưa xác minh"
	NTFromPhoto  = "Từ ảnh CTV"
)

type ContractorService struct {
	repo   repository.ContractorRepository
	houses repository.HouseRepository
	lark   LarkCTVSource // nil: không đồng bộ form
}

func NewContractorService(repo repository.ContractorRepository, houses repository.HouseRepository, src LarkCTVSource) *ContractorService {
	return &ContractorService{repo: repo, houses: houses, lark: src}
}

var nonDigit = regexp.MustCompile(`\D`)

// ntKey: SĐT chuẩn hoá (84xxx -> 0xxx) để khớp đăng ký với ảnh CTV.
func ntKey(phone string) string {
	d := nonDigit.ReplaceAllString(phone, "")
	if strings.HasPrefix(d, "84") && len(d) == 11 {
		d = "0" + d[2:]
	}
	return d
}

// SyncLark: 1 lượt đọc form đăng ký nhà thầu. Trả số dòng đã lưu.
func (s *ContractorService) SyncLark(ctx context.Context) (int, error) {
	if s.lark == nil {
		return 0, nil
	}
	recs, err := s.lark.Records(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range recs {
		f := r.Fields
		name := truncate(lark.Text(col(f, "ten cong ty")), 200)
		phone := normPhone(lark.Text(col(f, "so dien thoai")))
		if name == "" && phone == "" {
			continue
		}
		fields := []string{}
		if xs, ok := f["Lĩnh vực thi công"].([]any); ok {
			for _, x := range xs {
				if t := lark.Text(x); t != "" {
					fields = append(fields, t)
				}
			}
		}
		if other := lark.Text(f["Lĩnh vực thi công-Khác-Văn bản"]); other != "" {
			fields = append(fields, other)
		}
		vr := col(f, "xac minh")
		// ô "Xác minh tham gia": tích, hoặc chọn giá trị khác "chưa / không"
		t := strings.ToLower(lark.Text(vr))
		verified := lark.Checked(vr) || (t != "" && !strings.Contains(t, "chưa") && !strings.Contains(t, "không"))
		joined := time.Now()
		if ms, ok := col(f, "submitted on").(float64); ok && ms > 0 {
			joined = time.UnixMilli(int64(ms))
		}
		c := &repository.RegContractor{
			LarkRecord: r.ID, Name: name, Phone: phone,
			Kind:       truncate(lark.Text(col(f, "loai hinh")), 60),
			Role:       truncate(lark.Text(col(f, "vai tro")), 60),
			Contact:    truncate(lark.Text(col(f, "nguoi lien he")), 100),
			Email:      truncate(lark.Text(f["Email"]), 120),
			Areas:      truncate(lark.Text(col(f, "nhan thi cong")), 200),
			Exp:        truncate(lark.Text(col(f, "kinh nghiem")), 60),
			Staff:      truncate(lark.Text(col(f, "quy mo nhan su")), 60),
			ValueRange: truncate(lark.Text(col(f, "gia tri cong trinh")), 60),
			Fields:     fields, Verified: verified,
			Note:     truncate(lark.Text(f["Ghi chú"]), 500),
			JoinedAt: joined,
		}
		if err := s.repo.UpsertLark(ctx, c); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// StartLarkSync: đọc form đăng ký nhà thầu mỗi every (không có app Lark: không chạy).
func (s *ContractorService) StartLarkSync(every time.Duration) {
	if s.lark == nil {
		return
	}
	log.Printf("Nhà thầu từ form Lark: bật, mỗi %s", every)
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if _, err := s.SyncLark(ctx); err != nil {
				log.Printf("Nhà thầu từ form Lark: %v", err)
			}
			cancel()
			time.Sleep(every)
		}
	}()
}

func provNames(codes []string) string {
	out := []string{}
	for _, c := range codes {
		if p := province.ByCode(c); p != nil {
			out = append(out, p.Name)
		}
	}
	return strings.Join(out, ", ")
}

// rows: gộp 2 nguồn. raw: khoá SQL của các nhóm ảnh CTV theo khoá chuẩn hoá (để lấy công trình); prof: hồ sơ đăng ký.
func (s *ContractorService) rows(ctx context.Context) ([]ContractorRow, map[string][]string, map[string]*repository.RegContractor, error) {
	regs, err := s.repo.List(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	photo, err := s.houses.Contractors(ctx, "")
	if err != nil {
		return nil, nil, nil, err
	}
	byKey := map[string]*ContractorRow{}
	order := []string{}
	raw := map[string][]string{}
	prof := map[string]*repository.RegContractor{}
	for i := range regs {
		c := &regs[i]
		k := ntKey(c.Phone)
		if len(k) < 9 {
			k = "nt:" + strconv.FormatInt(c.ID, 10)
		}
		if _, dup := byKey[k]; dup {
			continue // cùng SĐT đăng ký nhiều lần: giữ lần mới nhất (List xếp mới trước)
		}
		st := NTUnverified
		if c.Verified {
			st = NTVerified
		}
		byKey[k] = &ContractorRow{Key: k, Name: c.Name, Kind: c.Kind, Role: c.Role, Phone: c.Phone, Areas: c.Areas, Exp: c.Exp,
			ValueRange: c.ValueRange, Status: st, Registered: true, Joined: c.JoinedAt, Updated: c.UpdatedAt}
		prof[k] = c
		order = append(order, k)
	}
	for _, p := range photo {
		k := p.Key
		if !strings.HasPrefix(k, "ten:") {
			k = ntKey(k)
		}
		raw[k] = append(raw[k], p.Key)
		r, ok := byKey[k]
		if !ok {
			r = &ContractorRow{Key: k, Name: p.Name, Phone: p.Phone, Status: NTFromPhoto, Joined: p.Last}
			byKey[k] = r
			order = append(order, k)
		}
		r.Photo = true
		r.Total += p.Total
		if r.Name == "" {
			r.Name = p.Name
		}
		if r.Areas == "" {
			r.Areas = provNames(p.Provinces)
		}
		if p.Last.After(r.Updated) {
			r.Updated = p.Last
		}
		if r.Joined.IsZero() || p.Last.Before(r.Joined) {
			r.Joined = p.Last
		}
	}
	out := make([]ContractorRow, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) }) // mới cập nhật trước
	return out, raw, prof, nil
}

func (s *ContractorService) List(ctx context.Context) ([]ContractorRow, error) {
	rs, _, _, err := s.rows(ctx)
	return rs, err
}

func (s *ContractorService) Get(ctx context.Context, key string) (*ContractorDetail, error) {
	rs, raw, prof, err := s.rows(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		if r.Key != key {
			continue
		}
		d := &ContractorDetail{ContractorRow: r, Profile: prof[key], Projects: []repository.SaleProject{}}
		for _, rk := range raw[key] {
			ps, _, err := s.houses.SaleProjects(ctx, repository.SaleFilter{Contractor: rk}, 500, 0)
			if err != nil {
				return nil, err
			}
			for i := range ps {
				ps[i].ProvinceName = provinceName(ps[i].Province)
			}
			d.Projects = append(d.Projects, ps...)
		}
		sort.SliceStable(d.Projects, func(i, j int) bool { return d.Projects[i].At.After(d.Projects[j].At) })
		return d, nil
	}
	return nil, fail(ErrNotFound, "Không tìm thấy nhà thầu.")
}
