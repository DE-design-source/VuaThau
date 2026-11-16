package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/pkg/xlsx"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/storage"
)

type Accounting struct {
	Ready   []model.PayRow `json:"ready"`
	Waiting []model.PayRow `json:"waiting"`
	Total   int64          `json:"total"`
}

type PayItem struct {
	CTVID int64 `json:"ctv_id"`
	Total int64 `json:"total"`
}

type PayManyResult struct {
	Message string  `json:"message"`
	Paid    []int64 `json:"paid"`
	Skipped int     `json:"skipped"`
}

type PayoutPage struct {
	User        *model.User        `json:"user"`
	Bank        model.Bank         `json:"bank"`
	Rows        []model.House      `json:"rows"`
	Adjustments []model.Adjustment `json:"adjustments"`
	Total       int64              `json:"total"`
	Memo        string             `json:"memo"`
}

type PayInput struct {
	HouseIDs      []int64 `json:"house_ids"`
	AdjustmentIDs []int64 `json:"adjustment_ids"`
	Ref           string  `json:"ref"`
}

type BatchList struct {
	Rows      []model.Batch `json:"rows"`
	TotalPaid int64         `json:"total_paid"`
}

type BatchDetail struct {
	Batch       *model.Batch       `json:"batch"`
	Rows        []model.House      `json:"rows"`
	Adjustments []model.Adjustment `json:"adjustments"`
	Logs        []model.Log        `json:"logs"`
	Total       int64              `json:"total"`
}

type Flow struct {
	Submitted int `json:"submitted"`
	Approved  int `json:"approved"`
	Returned  int `json:"returned"`
	Rejected  int `json:"rejected"`
}

type Report struct {
	From    string              `json:"from"`
	To      string              `json:"to"`
	Group   string              `json:"group"`
	Periods []repository.Period `json:"periods"`
	PerCTV  []repository.PerCTV `json:"per_ctv"`
	Flow    Flow                `json:"flow"`
	NewCTVs int                 `json:"new_ctvs"`
	Total   int64               `json:"total"`
	Batches int                 `json:"batches"`
	Houses  int                 `json:"houses"`
}

// Config: hằng số chung cho app và web (GET /config).
type Config struct {
	Statuses       map[string]model.StatusInfo `json:"statuses"`
	StatusOrder    []string                    `json:"status_order"`
	MinPhotos      int                         `json:"min_photos"`
	MaxPhotos      int                         `json:"max_photos"`
	PayMin         int                         `json:"pay_min"`
	Pay            []int64                     `json:"pay"`
	Stages         []string                    `json:"stages"`
	Banks          []string                    `json:"banks"`
	Reasons        map[string][]string         `json:"reasons"`
	GoogleClientID string                      `json:"google_client_id"`
}

type PaymentService interface {
	Config(ctx context.Context, googleClientID string) (*Config, error)
	Settings(ctx context.Context) (model.Settings, error)
	SaveSettings(ctx context.Context, by *model.User, s model.Settings) (string, model.Settings, error)

	Accounting(ctx context.Context) (*Accounting, error)
	Export(ctx context.Context, format string, ids []int64) ([]byte, string, string, error)
	PayMany(ctx context.Context, by *model.User, ref string, items []PayItem) (*PayManyResult, error)
	Payout(ctx context.Context, ctvID int64) (*PayoutPage, error)
	Pay(ctx context.Context, by *model.User, ctvID int64, in PayInput) (string, int64, error)

	Batches(ctx context.Context) (*BatchList, error)
	Batch(ctx context.Context, u *model.User, id int64) (*BatchDetail, error)
	SetRef(ctx context.Context, by *model.User, id int64, ref string) (string, error)
	Cancel(ctx context.Context, by *model.User, id int64, reason string) (string, error)
	UploadReceipt(ctx context.Context, by *model.User, id int64, data []byte) (string, error)
	Receipt(ctx context.Context, u *model.User, id int64) ([]byte, error)

	AddAdjustment(ctx context.Context, by *model.User, ctvID, amount int64, reason string) (string, error)
	DeleteAdjustment(ctx context.Context, by *model.User, id int64) (string, error)

	Report(ctx context.Context, from, to, group string) (*Report, error)
	ReportExport(ctx context.Context, from, to string) ([]byte, string, error)
}

type paymentService struct {
	payments repository.PaymentRepository
	houses   repository.HouseRepository
	photos   repository.PhotoRepository
	users    repository.UserRepository
	logs     repository.LogRepository
	settings repository.SettingsRepository
	files    *storage.Files
	now      func() time.Time
}

func NewPaymentService(payments repository.PaymentRepository, houses repository.HouseRepository, photos repository.PhotoRepository,
	users repository.UserRepository, logs repository.LogRepository, settings repository.SettingsRepository, files *storage.Files) PaymentService {
	return &paymentService{payments: payments, houses: houses, photos: photos, users: users, logs: logs, settings: settings,
		files: files, now: time.Now}
}

func (s *paymentService) logBatch(ctx context.Context, by *model.User, action string, bid int64, text string) {
	_ = s.logs.Add(ctx, by.ID, action, nil, &bid, text)
}

// ---------- Cấu hình ----------

func (s *paymentService) Config(ctx context.Context, googleClientID string) (*Config, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	pay := make([]int64, model.MaxPhotos+1)
	for n := range pay {
		pay[n] = st.Amount(n)
	}
	return &Config{Statuses: model.Statuses, StatusOrder: model.StatusOrder, MinPhotos: st.MinPhotos, MaxPhotos: model.MaxPhotos,
		PayMin: st.PayMin, Pay: pay, Stages: model.Stages, Banks: Banks, Reasons: Reasons, GoogleClientID: googleClientID}, nil
}

func (s *paymentService) Settings(ctx context.Context) (model.Settings, error) {
	return s.settings.Get(ctx)
}

func (s *paymentService) SaveSettings(ctx context.Context, by *model.User, n model.Settings) (string, model.Settings, error) {
	old, err := s.settings.Get(ctx)
	if err != nil {
		return "", old, err
	}
	switch {
	case n.MinPhotos < 1 || n.MinPhotos > model.MaxPhotos:
		return "", old, fail(ErrInvalidInput, "Chưa lưu: số ảnh đạt tối thiểu phải từ 1 đến %d.", model.MaxPhotos)
	case n.PriceMin < 0 || n.PriceMin > 10_000_000 || n.PriceMore < 0 || n.PriceMore > 10_000_000:
		return "", old, fail(ErrInvalidInput, "Chưa lưu: tiền 1 hồ sơ phải từ 0 đến 10.000.000đ.")
	case n.PayMin < 1 || n.PayMin > 100:
		return "", old, fail(ErrInvalidInput, "Chưa lưu: số hồ sơ để được thanh toán phải từ 1 đến 100.")
	}
	if n == old {
		return "Không có gì thay đổi.", old, nil
	}
	if err := s.settings.Save(ctx, n); err != nil {
		return "", old, err
	}
	diff := []string{}
	add := func(label string, a, b int64) {
		if a != b {
			diff = append(diff, fmt.Sprintf("%s: %d → %d", label, a, b))
		}
	}
	add(fmt.Sprintf("Tiền 1 hồ sơ đủ %d ảnh đạt", old.MinPhotos), old.PriceMin, n.PriceMin)
	add(fmt.Sprintf("Tiền 1 hồ sơ từ %d ảnh đạt", old.MinPhotos+1), old.PriceMore, n.PriceMore)
	add("Số ảnh đạt tối thiểu", int64(old.MinPhotos), int64(n.MinPhotos))
	add("Số hồ sơ để được thanh toán", int64(old.PayMin), int64(n.PayMin))
	_ = s.logs.Add(ctx, by.ID, ActSettings, nil, nil, strings.Join(diff, "; "))
	return "Đã lưu cấu hình. Giá mới áp dụng cho hồ sơ gửi duyệt từ bây giờ, hồ sơ đã gửi giữ giá cũ.", n, nil
}

// ---------- Kế toán ----------

func (s *paymentService) split(ctx context.Context) ([]model.PayRow, []model.PayRow, int64, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	rows, err := s.payments.Owed(ctx, 0)
	if err != nil {
		return nil, nil, 0, err
	}
	ready, waiting := []model.PayRow{}, []model.PayRow{}
	var total int64
	for _, r := range rows {
		if r.HouseCount >= st.PayMin && r.Total > 0 {
			ready = append(ready, r)
			total += r.Total
		} else {
			waiting = append(waiting, r)
		}
	}
	return ready, waiting, total, nil
}

func (s *paymentService) Accounting(ctx context.Context) (*Accounting, error) {
	ready, waiting, total, err := s.split(ctx)
	return &Accounting{Ready: ready, Waiting: waiting, Total: total}, err
}

func transferNote(username string, n int) string {
	return fmt.Sprintf("VUATHAU NGHIEM THU %s %d HS", username, n)
}

// Export: file chuyển khoản hàng loạt cho CTV đủ điều kiện (ids rỗng: tất cả). Trả nội dung, kiểu, tên file.
func (s *paymentService) Export(ctx context.Context, format string, ids []int64) ([]byte, string, string, error) {
	ready, _, _, err := s.split(ctx)
	if err != nil {
		return nil, "", "", err
	}
	pick := map[int64]bool{}
	for _, id := range ids {
		pick[id] = true
	}
	rows := []model.PayRow{}
	for _, r := range ready {
		if len(ids) == 0 || pick[r.CTVID] {
			rows = append(rows, r)
		}
	}
	if format == "csv" {
		var buf bytes.Buffer
		buf.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM: Excel mở đúng tiếng Việt
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"CTV", "SĐT", "Ngân hàng", "Số tài khoản", "Chủ tài khoản", "Số hồ sơ", "Số tiền", "Nội dung CK"})
		for _, r := range rows {
			b := BankParts(r.Bank)
			_ = w.Write([]string{r.Name, r.Username, b.Name, b.No, b.Owner, fmt.Sprint(r.HouseCount), fmt.Sprint(r.Total),
				transferNote(r.Username, r.HouseCount)})
		}
		w.Flush()
		return buf.Bytes(), "text/csv; charset=utf-8", "can-thanh-toan.csv", w.Error()
	}
	table := [][]any{{"STT", "Số tài khoản", "Tên người thụ hưởng", "Ngân hàng", "Số tiền", "Nội dung chuyển khoản", "CTV", "SĐT"}}
	for k, r := range rows {
		b := BankParts(r.Bank)
		owner := b.Owner
		if owner == "" {
			owner = r.Name
		}
		table = append(table, []any{k + 1, b.No, owner, b.Name, r.Total, transferNote(r.Username, r.HouseCount), r.Name, r.Username})
	}
	data, err := xlsx.Build(table)
	return data, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		fmt.Sprintf("chuyen-khoan-%s.xlsx", s.now().In(VN).Format("20060102")), err
}

func payError(err error) error {
	switch {
	case errors.Is(err, repository.ErrStale):
		return fail(ErrConflict, "Chưa ghi nhận: danh sách hồ sơ hoặc thưởng/trừ vừa thay đổi, kiểm tra lại.")
	case errors.Is(err, repository.ErrNotEnough):
		return fail(ErrInvalidInput, "Chưa ghi nhận: CTV chưa đủ số hồ sơ đã duyệt để thanh toán.")
	case errors.Is(err, repository.ErrZeroTotal):
		return fail(ErrInvalidInput, "Chưa ghi nhận: tổng tiền phải lớn hơn 0đ.")
	}
	return err
}

func (s *paymentService) PayMany(ctx context.Context, by *model.User, ref string, items []PayItem) (*PayManyResult, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	ref = truncate(ref, 100)
	res := &PayManyResult{Paid: []int64{}}
	var sum int64
	for _, it := range items {
		bid, total, err := s.payments.Pay(ctx, repository.PayInput{CTVID: it.CTVID, By: by.ID, Ref: ref, PayMin: st.PayMin,
			SeenTotal: ptr(it.Total)})
		if err != nil {
			if pe := payError(err); errors.Is(pe, ErrConflict) || errors.Is(pe, ErrInvalidInput) {
				res.Skipped++
				continue
			}
			return nil, err
		}
		res.Paid = append(res.Paid, bid)
		sum += total
	}
	res.Message = "Chưa ghi nhận phiếu chi nào."
	if len(res.Paid) > 0 {
		res.Message = fmt.Sprintf("Đã ghi nhận %d phiếu chi, tổng %s.", len(res.Paid), Money(sum))
	}
	if res.Skipped > 0 {
		res.Message += fmt.Sprintf(" Bỏ qua %d CTV vì số liệu vừa thay đổi hoặc chưa đủ điều kiện, kiểm tra lại.", res.Skipped)
	}
	return res, nil
}

func (s *paymentService) ctv(ctx context.Context, id int64) (*model.User, error) {
	u, err := s.users.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && u.Role != model.RoleCTV) {
		return nil, fail(ErrNotFound, "Không tìm thấy cộng tác viên.")
	}
	return u, err
}

func (s *paymentService) Payout(ctx context.Context, ctvID int64) (*PayoutPage, error) {
	u, err := s.ctv(ctx, ctvID)
	if err != nil {
		return nil, err
	}
	p := &PayoutPage{User: u, Bank: BankParts(u.Bank)}
	if p.Rows, err = s.houses.List(ctx, repository.HouseFilter{CTVID: ctvID, Statuses: []string{model.StatusApproved}}, 1000, 0); err != nil {
		return nil, err
	}
	if err := withStats(ctx, s.photos, p.Rows); err != nil {
		return nil, err
	}
	if p.Adjustments, err = s.payments.Adjustments(ctx, ctvID, true); err != nil {
		return nil, err
	}
	for _, h := range p.Rows {
		p.Total += h.Amount
	}
	for _, a := range p.Adjustments {
		p.Total += a.Amount
	}
	p.Memo = transferNote(u.Username, len(p.Rows))
	return p, nil
}

func (s *paymentService) Pay(ctx context.Context, by *model.User, ctvID int64, in PayInput) (string, int64, error) {
	if _, err := s.ctv(ctx, ctvID); err != nil {
		return "", 0, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return "", 0, err
	}
	if in.HouseIDs == nil {
		in.HouseIDs = []int64{}
	}
	if in.AdjustmentIDs == nil {
		in.AdjustmentIDs = []int64{}
	}
	bid, total, err := s.payments.Pay(ctx, repository.PayInput{CTVID: ctvID, By: by.ID, Ref: truncate(in.Ref, 100),
		PayMin: st.PayMin, HouseIDs: in.HouseIDs, AdjIDs: in.AdjustmentIDs})
	if err != nil {
		return "", 0, payError(err)
	}
	return fmt.Sprintf("Đã ghi nhận thanh toán %s.", Money(total)), bid, nil
}

func (s *paymentService) Batches(ctx context.Context) (*BatchList, error) {
	rows, err := s.payments.Batches(ctx, ListLimit)
	if err != nil {
		return nil, err
	}
	total, err := s.payments.TotalPaid(ctx)
	return &BatchList{Rows: rows, TotalPaid: total}, err
}

// batchOf: phiếu chi + quyền xem. Nhân viên xem cả phiếu đã huỷ, CTV chỉ phiếu chưa huỷ có hồ sơ của mình.
func (s *paymentService) batchOf(ctx context.Context, u *model.User, id int64) (*model.Batch, error) {
	b, err := s.payments.Batch(ctx, id)
	notFound := fail(ErrNotFound, "Không tìm thấy khoản thanh toán.")
	if errors.Is(err, repository.ErrNotFound) {
		return nil, notFound
	}
	if err != nil {
		return nil, err
	}
	if u.Role == model.RoleCTV && (b.Status != model.BatchPaid || b.CTVID == nil || *b.CTVID != u.ID) {
		return nil, notFound
	}
	return b, nil
}

func (s *paymentService) Batch(ctx context.Context, u *model.User, id int64) (*BatchDetail, error) {
	b, err := s.batchOf(ctx, u, id)
	if err != nil {
		return nil, err
	}
	d := &BatchDetail{Batch: b, Logs: []model.Log{}}
	f := repository.HouseFilter{BatchID: id}
	var onlyCTV int64
	if u.Role == model.RoleCTV {
		f.CTVID, onlyCTV = u.ID, u.ID
	}
	if d.Rows, err = s.houses.List(ctx, f, 1000, 0); err != nil {
		return nil, err
	}
	if err := withStats(ctx, s.photos, d.Rows); err != nil {
		return nil, err
	}
	if d.Adjustments, err = s.payments.BatchAdjustments(ctx, id, onlyCTV); err != nil {
		return nil, err
	}
	if u.IsStaff() { // lịch sử phiếu chi chỉ cho nhân viên
		if d.Logs, err = s.logs.ForBatch(ctx, id); err != nil {
			return nil, err
		}
	}
	if b.Status == model.BatchPaid {
		for _, h := range d.Rows {
			d.Total += h.Amount
		}
		for _, a := range d.Adjustments {
			d.Total += a.Amount
		}
	} else {
		d.Total = b.Total
	}
	return d, nil
}

func (s *paymentService) SetRef(ctx context.Context, by *model.User, id int64, ref string) (string, error) {
	b, err := s.batchOf(ctx, by, id)
	if err != nil {
		return "", err
	}
	ref = truncate(ref, 100)
	if err := s.payments.SetRef(ctx, id, ref); err != nil {
		if errors.Is(err, repository.ErrNotPayable) {
			return "", fail(ErrForbidden, "Phiếu chi đã huỷ, không sửa được.")
		}
		return "", err
	}
	orEmpty := func(s string) string {
		if s == "" {
			return "(trống)"
		}
		return s
	}
	s.logBatch(ctx, by, ActRef, id, orEmpty(b.Ref)+" → "+orEmpty(ref))
	return "Đã sửa mã giao dịch.", nil
}

func (s *paymentService) Cancel(ctx context.Context, by *model.User, id int64, reason string) (string, error) {
	reason = truncate(reason, 300)
	if reason == "" {
		return "", fail(ErrInvalidInput, "Huỷ phiếu chi cần ghi lý do.")
	}
	if _, err := s.batchOf(ctx, by, id); err != nil {
		return "", err
	}
	hids, err := s.payments.Cancel(ctx, id)
	if errors.Is(err, repository.ErrNotPayable) {
		return "", fail(ErrConflict, "Phiếu chi này đã huỷ rồi.")
	}
	if err != nil {
		return "", err
	}
	tags := make([]string, len(hids))
	for i, h := range hids {
		tags[i] = fmt.Sprintf("#%d", h)
		_ = s.logs.Add(ctx, by.ID, ActUnpay, &hids[i], nil, fmt.Sprintf("phiếu chi #%d: %s", id, reason))
	}
	s.logBatch(ctx, by, ActCancel, id, reason+" · hồ sơ "+strings.Join(tags, ", "))
	return fmt.Sprintf("Đã huỷ phiếu chi #%d. %d hồ sơ về lại Chờ thanh toán.", id, len(hids)), nil
}

func (s *paymentService) UploadReceipt(ctx context.Context, by *model.User, id int64, data []byte) (string, error) {
	b, err := s.batchOf(ctx, by, id)
	if err != nil {
		return "", err
	}
	if len(data) > MaxFileBytes {
		return "", fail(ErrInvalidInput, "Chưa lưu: chỉ nhận ảnh JPG/PNG dưới 20MB (chụp hoặc chụp màn hình biên lai).")
	}
	img, _, _, err := imaging.Open(data)
	if err != nil {
		return "", fail(ErrInvalidInput, "Chưa lưu: chỉ nhận ảnh JPG/PNG dưới 20MB (chụp hoặc chụp màn hình biên lai).")
	}
	jpg, err := imaging.SmallJPEG(img, ReceiptSide)
	if err != nil {
		return "", err
	}
	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	name := fmt.Sprintf("%d-%s.jpg", id, hex.EncodeToString(rnd))
	if err := s.files.SaveReceipt(name, jpg); err != nil {
		return "", err
	}
	if err := s.payments.SetReceipt(ctx, id, name); err != nil {
		return "", err
	}
	if b.Receipt != "" {
		s.files.DeleteReceipt(b.Receipt)
	}
	s.logBatch(ctx, by, ActReceipt, id, "")
	return "Đã lưu ảnh biên lai.", nil
}

func (s *paymentService) Receipt(ctx context.Context, u *model.User, id int64) ([]byte, error) {
	b, err := s.batchOf(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if b.Receipt == "" {
		return nil, fail(ErrNotFound, "Chưa có ảnh biên lai.")
	}
	data, err := s.files.ReadReceipt(b.Receipt)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file biên lai.")
	}
	return data, nil
}

func (s *paymentService) AddAdjustment(ctx context.Context, by *model.User, ctvID, amount int64, reason string) (string, error) {
	u, err := s.ctv(ctx, ctvID)
	if err != nil {
		return "", err
	}
	reason = truncate(reason, 300)
	if amount == 0 || amount > 99_999_999 || amount < -99_999_999 || reason == "" {
		return "", fail(ErrInvalidInput, "Chưa thêm: nhập số tiền (thưởng là số dương, trừ là số âm, vd -20000) và lý do.")
	}
	if err := s.payments.AddAdjustment(ctx, &model.Adjustment{CTVID: ctvID, Amount: amount, Note: reason, CreatedBy: &by.ID}); err != nil {
		return "", err
	}
	_ = s.logs.Add(ctx, by.ID, ActAdjust, nil, nil, fmt.Sprintf("%s: %s, %s", u.Name, Money(amount), reason))
	word := "thưởng"
	if amount < 0 {
		word, amount = "trừ", -amount
	}
	return fmt.Sprintf("Đã %s %s cho %s, tính vào lần thanh toán tới.", word, Money(amount), u.Name), nil
}

func (s *paymentService) DeleteAdjustment(ctx context.Context, by *model.User, id int64) (string, error) {
	a, err := s.payments.Adjustment(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return "", fail(ErrNotFound, "Không tìm thấy khoản thưởng / trừ.")
	}
	if err != nil {
		return "", err
	}
	if a.BatchID != nil {
		return "", fail(ErrForbidden, "Khoản này đã được thanh toán, không xoá được.")
	}
	if err := s.payments.DeleteAdjustment(ctx, id); err != nil {
		return "", err
	}
	name := ""
	if u, err := s.users.GetByID(ctx, a.CTVID); err == nil {
		name = u.Name
	}
	_ = s.logs.Add(ctx, by.ID, ActAdjDel, nil, nil, fmt.Sprintf("%s: %s, %s", name, Money(a.Amount), a.Note))
	return "Đã xoá khoản thưởng / trừ.", nil
}

// ---------- Báo cáo ----------

// Biểu thức nhóm theo giờ Việt Nam (cố định, không lấy từ người dùng).
var groupSQL = map[string]string{
	"day":   `to_char(b.created_at AT TIME ZONE 'Asia/Ho_Chi_Minh', 'YYYY-MM-DD')`,
	"week":  `to_char(b.created_at AT TIME ZONE 'Asia/Ho_Chi_Minh', 'IYYY-"W"IW')`,
	"month": `to_char(b.created_at AT TIME ZONE 'Asia/Ho_Chi_Minh', 'YYYY-MM')`,
}

// reportRange: mặc định từ đầu tháng tới hôm nay. Trả chuỗi ngày và khoảng [from, to+1 ngày).
func (s *paymentService) reportRange(from, to string) (string, string, time.Time, time.Time) {
	today := s.now().In(VN)
	if !ValidDay(from) {
		from = today.Format("2006-01") + "-01"
	}
	if !ValidDay(to) {
		to = today.Format("2006-01-02")
	}
	if from > to {
		from, to = to, from
	}
	f, _ := time.ParseInLocation("2006-01-02", from, VN)
	t, _ := time.ParseInLocation("2006-01-02", to, VN)
	return from, to, f, t.AddDate(0, 0, 1)
}

func (s *paymentService) Report(ctx context.Context, from, to, group string) (*Report, error) {
	if _, ok := groupSQL[group]; !ok {
		group = "day"
	}
	from, to, f, t := s.reportRange(from, to)
	r := &Report{From: from, To: to, Group: group}
	var err error
	if r.Periods, err = s.payments.Periods(ctx, groupSQL[group], f, t); err != nil {
		return nil, err
	}
	if r.PerCTV, err = s.payments.PerCTV(ctx, f, t); err != nil {
		return nil, err
	}
	flow, err := s.logs.Flow(ctx, []string{ActSubmit, ActApprove, ActReturn, ActReject}, f, t)
	if err != nil {
		return nil, err
	}
	r.Flow = Flow{Submitted: flow[ActSubmit], Approved: flow[ActApprove], Returned: flow[ActReturn], Rejected: flow[ActReject]}
	if r.NewCTVs, err = s.users.CountNewCTVs(ctx, f, t); err != nil {
		return nil, err
	}
	for _, p := range r.Periods {
		r.Total += p.Total
		r.Batches += p.Batches
		r.Houses += p.Houses
	}
	return r, nil
}

func (s *paymentService) ReportExport(ctx context.Context, from, to string) ([]byte, string, error) {
	from, to, f, t := s.reportRange(from, to)
	lines, err := s.payments.PaidLines(ctx, f, t)
	if err != nil {
		return nil, "", err
	}
	table := [][]any{{"Ngày chi", "Phiếu chi", "CTV", "SĐT", "Ngân hàng", "Số tài khoản", "Chủ tài khoản", "Khoản",
		"Địa chỉ / lý do", "Số tiền", "Mã giao dịch"}}
	var sum int64
	for _, l := range lines {
		b := BankParts(l.Bank)
		table = append(table, []any{l.At.In(VN).Format("2006-01-02 15:04"), fmt.Sprintf("#%d", l.BatchID), l.Name, l.Username,
			b.Name, b.No, b.Owner, l.Item, l.Detail, l.Amount, l.Ref})
		sum += l.Amount
	}
	table = append(table, []any{"Tổng", "", "", "", "", "", "", fmt.Sprintf("%d khoản", len(lines)), "", sum, ""})
	data, err := xlsx.Build(table)
	return data, fmt.Sprintf("bao-cao-chi-%s-den-%s.xlsx", from, to), err
}
