package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"vuathau/backend/internal/model"
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
	User        *model.User        `json:"user"`
	Bank        model.Bank         `json:"bank"`
	Money       model.Money        `json:"money"`
	Rows        []model.House      `json:"rows"`
	Counts      map[string]int     `json:"counts"`
	Pays        []model.Payout     `json:"pays"`
	Adjustments []model.Adjustment `json:"adjustments"`
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
	GetByID(ctx context.Context, id int64) (*model.User, error)
	List(ctx context.Context, limit, offset int) ([]model.User, error)

	Me(ctx context.Context, u *model.User) (*MeResult, error)
	UpdateBank(ctx context.Context, u *model.User, name, no, owner string) (*model.User, error)
	ChangePassword(ctx context.Context, u *model.User, old, new string) (*model.User, error)
	DeleteSelf(ctx context.Context, u *model.User) (string, error)

	ListStaff(ctx context.Context) ([]model.User, error)
	AddStaff(ctx context.Context, by *model.User, in AddStaffInput) (*model.User, string, error)
	AddCTV(ctx context.Context, by *model.User, in AddCTVInput) (*model.User, string, error)
	SetLocked(ctx context.Context, by *model.User, id int64, locked bool) (string, error)
	ResetPassword(ctx context.Context, by *model.User, id int64, pw string) (string, error)

	ListCTVs(ctx context.Context, q string, page int) ([]repository.CTVRow, int, int, error)
	CTV(ctx context.Context, id int64) (*CTVDetail, error)
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

func (s *userService) GetByID(ctx context.Context, id int64) (*model.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *userService) List(ctx context.Context, limit, offset int) ([]model.User, error) {
	limit, offset = normalizePage(limit, offset)
	return s.repo.List(ctx, limit, offset)
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

var roleLabel = map[string]string{model.RoleCTV: "cộng tác viên", model.RoleAdmin: "admin", model.RoleKetoan: "kế toán"}

func (s *userService) AddStaff(ctx context.Context, by *model.User, in AddStaffInput) (*model.User, string, error) {
	login, name := LoginID(in.Login), truncate(in.Name, 100)
	if (in.Role != model.RoleAdmin && in.Role != model.RoleKetoan) || name == "" || len(in.Password) < 8 || len(in.Password) > 72 ||
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

func (s *userService) ListCTVs(ctx context.Context, q string, page int) ([]repository.CTVRow, int, int, error) {
	page = max(1, page)
	rows, found, err := s.repo.ListCTVs(ctx, strings.TrimSpace(q), PageSize, (page-1)*PageSize)
	return rows, found, page, err
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
	d.Adjustments, err = s.payments.Adjustments(ctx, id, false)
	return d, err
}
