package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// MeResult: GET /me.
type MeResult struct {
	User  *model.User  `json:"user"`
	Bank  model.Bank   `json:"bank"`
	Money *model.Money `json:"money,omitempty"`
	Todo  int          `json:"todo"`
}

// CTVDetail: 1 CTV cho admin / kế toán.
type CTVDetail struct {
	User        *model.User            `json:"user"`
	Bank        model.Bank             `json:"bank"`
	Money       model.Money            `json:"money"`
	Rows        []model.House          `json:"rows"`
	Counts      map[string]int         `json:"counts"`
	Pays        []model.Payout         `json:"pays"`
	Adjustments []model.Adjustment     `json:"adjustments"`
	Profile     *repository.CTVProfile `json:"profile"`
}

type AddStaffInput struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

// AddCTVInput: admin tạo tài khoản CTV (đăng nhập app bằng SĐT + mật khẩu). Ngân hàng không bắt buộc:
// bỏ trống cả 3 ô thì CTV tự thêm trong app (Tài khoản) trước khi nhận tiền.
type AddCTVInput struct {
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Password  string `json:"password"`
	BankName  string `json:"bank_name"`
	BankNo    string `json:"bank_no"`
	BankOwner string `json:"bank_owner"`
}

type UserService interface {
	Me(ctx context.Context, u *model.User) (*MeResult, error)
	UpdateBank(ctx context.Context, u *model.User, name, no, owner string) (*model.User, error)
	ChangePassword(ctx context.Context, u *model.User, old, new string) (*model.User, error)
	DeleteSelf(ctx context.Context, u *model.User) (string, error)

	ListStaff(ctx context.Context) ([]model.User, error)
	AddStaff(ctx context.Context, by *model.User, in AddStaffInput) (*model.User, string, error)
	AddCTV(ctx context.Context, by *model.User, in AddCTVInput) (*model.User, string, error)
	SetLocked(ctx context.Context, by *model.User, id int64, locked bool) (string, error)
	ResetPassword(ctx context.Context, by *model.User, id int64, pw string) (string, error)

	ListCTVs(ctx context.Context, f repository.CTVFilter, page int) ([]repository.CTVRow, int, int, error)
	CTVFacets(ctx context.Context) (*CTVFacets, error)
	CTV(ctx context.Context, id int64) (*CTVDetail, error)
	SetCTVProfile(ctx context.Context, by *model.User, id int64, in CTVProfileInput) (string, error)
	SaveKinds(ctx context.Context, by *model.User, kinds []string) ([]string, string, error)
}

// CTVFacets: lựa chọn của các ô lọc trang Cộng tác viên, kèm số CTV mỗi lựa chọn.
type CTVFacets struct {
	Regions []Facet `json:"regions"` // tỉnh có CTV (theo tên); value "-" = chưa rõ khu vực
	Kinds   []Facet `json:"kinds"`   // danh sách chức năng + loại cũ CTV còn mang
}

type Facet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// CTVProfileInput: admin sửa khu vực (mã tỉnh, '' = tự tính), chức năng, ghi chú của 1 CTV.
type CTVProfileInput struct {
	Region string `json:"region"`
	Kind   string `json:"kind"`
	Note   string `json:"note"`
}

type userService struct {
	repo     repository.UserRepository
	auth     repository.AuthRepository
	houses   repository.HouseRepository
	photos   repository.PhotoRepository
	payments repository.PaymentRepository
	logs     repository.LogRepository
	settings repository.SettingsRepository
}

func NewUserService(repo repository.UserRepository, auth repository.AuthRepository, houses repository.HouseRepository,
	photos repository.PhotoRepository, payments repository.PaymentRepository, logs repository.LogRepository,
	settings repository.SettingsRepository) UserService {
	return &userService{repo: repo, auth: auth, houses: houses, photos: photos, payments: payments, logs: logs, settings: settings}
}

// Todo: số việc đang chờ vai trò này (CTV: nháp + trả lại; admin: chờ duyệt; kế toán: CTV đủ điều kiện).
func todoOf(ctx context.Context, u *model.User, houses repository.HouseRepository, payments repository.PaymentRepository,
	settings repository.SettingsRepository) (int, error) {
	switch u.Role {
	case model.RoleAdmin:
		return houses.Count(ctx, repository.HouseFilter{Statuses: []string{model.StatusPending}})
	case model.RoleKetoan:
		st, err := settings.Get(ctx)
		if err != nil {
			return 0, err
		}
		return payments.CountReady(ctx, st.PayMin)
	case model.RoleSale:
		return 0, nil
	default:
		return houses.CTVTodo(ctx, u.ID)
	}
}

func (s *userService) Me(ctx context.Context, u *model.User) (*MeResult, error) {
	res := &MeResult{User: u, Bank: BankParts(u.Bank)}
	var err error
	if res.Todo, err = todoOf(ctx, u, s.houses, s.payments, s.settings); err != nil {
		return nil, err
	}
	if u.Role == model.RoleCTV {
		m, err := s.houses.CTVMoney(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		res.Money = &m
	}
	return res, nil
}

func (s *userService) UpdateBank(ctx context.Context, u *model.User, name, no, owner string) (*model.User, error) {
	if u.Role != model.RoleCTV {
		return nil, ErrForbidden
	}
	bank := BankFrom(name, no, owner)
	if bank == "" {
		return nil, fail(ErrInvalidInput, "Chưa lưu: cần đủ ngân hàng, số tài khoản (6-20 chữ số) và tên chủ tài khoản.")
	}
	if err := s.repo.UpdateBank(ctx, u.ID, bank); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, u.ID)
}

func (s *userService) ChangePassword(ctx context.Context, u *model.User, old, new string) (*model.User, error) {
	if !checkPassword(u.PasswordHash, old) || utf8.RuneCountInString(new) < 6 || len(new) > 72 {
		return nil, fail(ErrInvalidInput, "Chưa đổi được: mật khẩu hiện tại sai, hoặc mật khẩu mới dưới 6 ký tự.")
	}
	hash, err := hashPassword(new)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdatePassword(ctx, u.ID, hash); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, u.ID)
}

func randomSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// DeleteSelf: CTV tự xoá tài khoản (CH Play bắt buộc). Hồ sơ, phiếu chi giữ dưới tên ẩn danh để đối soát.
func (s *userService) DeleteSelf(ctx context.Context, u *model.User) (string, error) {
	if u.Role != model.RoleCTV {
		return "", ErrForbidden
	}
	m, err := s.houses.CTVMoney(ctx, u.ID)
	if err != nil {
		return "", err
	}
	hash, err := hashPassword(randomSecret())
	if err != nil {
		return "", err
	}
	if err := s.repo.Anonymize(ctx, u.ID, hash); err != nil {
		return "", err
	}
	note := ""
	if m.Owed > 0 {
		note = "còn " + Money(m.Owed) + " chưa trả"
	}
	_ = s.logs.Add(ctx, u.ID, ActSelfDel, nil, nil, note)
	return "Đã xoá tài khoản và thông tin cá nhân của bạn. Cảm ơn bạn đã cộng tác với Vua Thầu.", nil
}

func (s *userService) ListStaff(ctx context.Context) ([]model.User, error) {
	return s.repo.ListStaff(ctx)
}

var roleLabel = map[string]string{model.RoleCTV: "cộng tác viên", model.RoleAdmin: "admin", model.RoleKetoan: "kế toán", model.RoleSale: "sale"}

func (s *userService) AddStaff(ctx context.Context, by *model.User, in AddStaffInput) (*model.User, string, error) {
	login, name := LoginID(in.Login), truncate(in.Name, 100)
	if (in.Role != model.RoleAdmin && in.Role != model.RoleKetoan && in.Role != model.RoleSale) || name == "" || len(in.Password) < 8 || len(in.Password) > 72 ||
		!(ValidPhone(login) || staffLogin.MatchString(login)) {
		return nil, "", fail(ErrInvalidInput, "Chưa thêm: tên đăng nhập 3-30 ký tự a-z, 0-9, _ (hoặc SĐT), có họ tên, mật khẩu từ 8 ký tự.")
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, "", err
	}
	u := &model.User{Username: login, Name: name, Role: in.Role, PasswordHash: hash}
	if err := s.auth.CreateUser(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, "", fail(ErrConflict, "Tên đăng nhập %s đã có người dùng.", login)
		}
		return nil, "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActAddStaff, nil, nil, fmt.Sprintf("%s (%s, %s)", name, login, roleLabel[in.Role]))
	return u, fmt.Sprintf("Đã thêm %s %s, đăng nhập bằng %s.", roleLabel[in.Role], name, login), nil
}

func (s *userService) AddCTV(ctx context.Context, by *model.User, in AddCTVInput) (*model.User, string, error) {
	phone, name := normPhone(in.Phone), truncate(in.Name, 100)
	if name == "" || !ValidPhone(phone) {
		return nil, "", fail(ErrInvalidInput, "Chưa tạo: cần họ tên và SĐT hợp lệ (10-11 số, bắt đầu bằng 0).")
	}
	if utf8.RuneCountInString(in.Password) < 6 || len(in.Password) > 72 {
		return nil, "", fail(ErrInvalidInput, "Chưa tạo: mật khẩu cần ít nhất 6 ký tự.")
	}
	bank := ""
	if strings.TrimSpace(in.BankName+in.BankNo+in.BankOwner) != "" {
		if bank = BankFrom(in.BankName, in.BankNo, in.BankOwner); bank == "" {
			return nil, "", fail(ErrInvalidInput, "Chưa tạo: ngân hàng cần đủ tên ngân hàng, số tài khoản (chỉ số) và tên chủ tài khoản, hoặc bỏ trống cả 3 ô.")
		}
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, "", err
	}
	u := &model.User{Username: phone, Name: name, Role: model.RoleCTV, PasswordHash: hash, Bank: bank}
	if err := s.auth.CreateUser(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			if old, e := s.auth.FindByLogin(ctx, phone); e == nil && old.Role == model.RoleCTV {
				return nil, "", fail(ErrConflict, "SĐT %s đã có tài khoản CTV %s. Mở trang CTV đó để đặt mật khẩu mới.", phone, old.Name)
			}
			return nil, "", fail(ErrConflict, "SĐT %s đã được dùng cho một tài khoản khác.", phone)
		}
		return nil, "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActAddCTV, nil, nil, fmt.Sprintf("%s (%s)", name, phone))
	return u, fmt.Sprintf("Đã tạo tài khoản CTV %s. Đăng nhập app bằng SĐT %s và mật khẩu vừa đặt.", name, phone), nil
}

func (s *userService) target(ctx context.Context, by *model.User, id int64) (*model.User, error) {
	x, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrNotFound, "Không tìm thấy tài khoản.")
	}
	if err != nil {
		return nil, err
	}
	if x.Locked == model.LockedDeleted {
		return nil, fail(ErrForbidden, "CTV đã tự xoá tài khoản này.")
	}
	return x, nil
}

func (s *userService) SetLocked(ctx context.Context, by *model.User, id int64, locked bool) (string, error) {
	if id == by.ID {
		return "", fail(ErrForbidden, "Không tự khoá tài khoản của chính mình.")
	}
	x, err := s.target(ctx, by, id)
	if err != nil {
		return "", err
	}
	v, act, word := model.LockedNone, ActUnlock, "mở khoá"
	if locked {
		v, act, word = model.LockedByAdmin, ActLock, "khoá"
	}
	if err := s.repo.SetLocked(ctx, id, v); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, by.ID, act, nil, nil, fmt.Sprintf("%s (%s)", x.Name, x.Username))
	return fmt.Sprintf("Đã %s tài khoản %s.", word, x.Name), nil
}

func (s *userService) ResetPassword(ctx context.Context, by *model.User, id int64, pw string) (string, error) {
	x, err := s.target(ctx, by, id)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(pw) < 6 || len(pw) > 72 {
		return "", fail(ErrInvalidInput, "Mật khẩu mới cần ít nhất 6 ký tự.")
	}
	hash, err := hashPassword(pw)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdatePassword(ctx, id, hash); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActResetPW, nil, nil, fmt.Sprintf("%s (%s)", x.Name, x.Username))
	return fmt.Sprintf("Đã đặt mật khẩu mới cho %s. Báo họ đăng nhập lại bằng mật khẩu này.", x.Name), nil
}

func pages(found int) int { return max(1, (found+PageSize-1)/PageSize) }

func (s *userService) ListCTVs(ctx context.Context, f repository.CTVFilter, page int) ([]repository.CTVRow, int, int, error) {
	page = max(1, page)
	f.Q = strings.TrimSpace(f.Q)
	if f.Bank != "co" && f.Bank != "chua" {
		f.Bank = ""
	}
	rows, found, err := s.repo.ListCTVs(ctx, f, PageSize, (page-1)*PageSize)
	return rows, found, page, err
}

func (s *userService) CTVFacets(ctx context.Context) (*CTVFacets, error) {
	regions, kinds, err := s.repo.CTVFacets(ctx)
	if err != nil {
		return nil, err
	}
	res := &CTVFacets{Regions: []Facet{}, Kinds: []Facet{}}
	for code, n := range regions {
		if code == "" {
			continue
		}
		label := code
		if p := province.ByCode(code); p != nil {
			label = p.Name
		}
		res.Regions = append(res.Regions, Facet{Value: code, Label: label, Count: n})
	}
	slices.SortFunc(res.Regions, func(a, b Facet) int { return strings.Compare(a.Label, b.Label) })
	if n := regions[""]; n > 0 {
		res.Regions = append(res.Regions, Facet{Value: "-", Label: "Chưa rõ khu vực", Count: n})
	}
	list, err := s.settings.Kinds(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range list {
		res.Kinds = append(res.Kinds, Facet{Value: k, Label: k, Count: kinds[k]})
	}
	for k, n := range kinds { // loại đã bỏ khỏi danh sách nhưng còn CTV mang
		if !slices.Contains(list, k) {
			res.Kinds = append(res.Kinds, Facet{Value: k, Label: k, Count: n})
		}
	}
	return res, nil
}

func (s *userService) SetCTVProfile(ctx context.Context, by *model.User, id int64, in CTVProfileInput) (string, error) {
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && u.Role != model.RoleCTV) {
		return "", fail(ErrNotFound, "Không tìm thấy cộng tác viên.")
	}
	if err != nil {
		return "", err
	}
	old, err := s.repo.Profile(ctx, id)
	if err != nil {
		return "", err
	}
	in.Region, in.Kind, in.Note = strings.TrimSpace(in.Region), strings.TrimSpace(in.Kind), strings.TrimSpace(in.Note)
	if in.Region != "" && province.ByCode(in.Region) == nil {
		return "", fail(ErrInvalidInput, "Khu vực không hợp lệ.")
	}
	if in.Kind == "" {
		in.Kind = old.Kind
	}
	if in.Kind != old.Kind {
		kinds, err := s.settings.Kinds(ctx)
		if err != nil {
			return "", err
		}
		if !slices.Contains(kinds, in.Kind) {
			return "", fail(ErrInvalidInput, "Chức năng không có trong danh sách (sửa danh sách ở Cấu hình).")
		}
	}
	if utf8.RuneCountInString(in.Note) > 1000 {
		return "", fail(ErrInvalidInput, "Ghi chú dài quá 1000 ký tự.")
	}
	if err := s.repo.SetProfile(ctx, id, in.Region, in.Kind, in.Note); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActCTVProfile, nil, nil, fmt.Sprintf("%s (%s): khu vực %s, chức năng %s", u.Name, u.Username,
		regionText(in.Region), in.Kind))
	return fmt.Sprintf("Đã lưu thông tin %s.", u.Name), nil
}

func regionText(code string) string {
	if p := province.ByCode(code); p != nil {
		return p.Name
	}
	return "tự tính"
}

// SaveKinds: admin sửa danh sách chức năng CTV (bỏ trùng, bỏ ô trống, tối đa 20 loại, mỗi loại tối đa 40 ký tự).
func (s *userService) SaveKinds(ctx context.Context, by *model.User, in []string) ([]string, string, error) {
	kinds := []string{}
	for _, k := range in {
		k = strings.Join(strings.Fields(k), " ")
		if k == "" || slices.Contains(kinds, k) {
			continue
		}
		if utf8.RuneCountInString(k) > 40 {
			return nil, "", fail(ErrInvalidInput, "Tên chức năng dài quá 40 ký tự: %s", k)
		}
		kinds = append(kinds, k)
	}
	if len(kinds) == 0 {
		return nil, "", fail(ErrInvalidInput, "Cần ít nhất 1 chức năng.")
	}
	if len(kinds) > 20 {
		return nil, "", fail(ErrInvalidInput, "Tối đa 20 chức năng.")
	}
	if err := s.settings.SaveKinds(ctx, kinds); err != nil {
		return nil, "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActSettings, nil, nil, "Chức năng CTV: "+strings.Join(kinds, ", "))
	return kinds, "Đã lưu danh sách chức năng.", nil
}

func (s *userService) CTV(ctx context.Context, id int64) (*CTVDetail, error) {
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && u.Role != model.RoleCTV) {
		return nil, fail(ErrNotFound, "Không tìm thấy cộng tác viên.")
	}
	if err != nil {
		return nil, err
	}
	d := &CTVDetail{User: u, Bank: BankParts(u.Bank)}
	if d.Money, err = s.houses.CTVMoney(ctx, id); err != nil {
		return nil, err
	}
	if d.Rows, err = s.houses.List(ctx, repository.HouseFilter{CTVID: id}, ListLimit, 0); err != nil {
		return nil, err
	}
	if err := withStats(ctx, s.photos, d.Rows); err != nil {
		return nil, err
	}
	if d.Counts, err = s.houses.CTVStatusCounts(ctx, id); err != nil {
		return nil, err
	}
	if d.Pays, err = s.payments.Payouts(ctx, id); err != nil {
		return nil, err
	}
	if d.Adjustments, err = s.payments.Adjustments(ctx, id, false); err != nil {
		return nil, err
	}
	d.Profile, err = s.repo.Profile(ctx, id)
	return d, err
}
