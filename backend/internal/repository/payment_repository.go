package repository

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

// Lỗi khi lập phiếu chi.
var (
	ErrStale      = errors.New("danh sách hồ sơ hoặc thưởng/trừ vừa thay đổi")
	ErrNotEnough  = errors.New("chưa đủ số hồ sơ đã duyệt")
	ErrZeroTotal  = errors.New("tổng tiền phải lớn hơn 0")
	ErrNotPayable = errors.New("phiếu chi đã huỷ")
)

// PayInput: những gì kế toán nhìn thấy lúc bấm trả. Nil = không đối chiếu.
type PayInput struct {
	CTVID     int64
	By        int64
	Ref       string
	PayMin    int
	HouseIDs  []int64
	AdjIDs    []int64
	SeenTotal *int64
}

// Period: 1 dòng báo cáo theo ngày / tuần / tháng.
type Period struct {
	Key     string `json:"key"`
	Batches int    `json:"batches"`
	CTVs    int    `json:"ctvs"`
	Houses  int    `json:"houses"`
	Total   int64  `json:"total"`
}

// PerCTV: 1 dòng báo cáo theo CTV.
type PerCTV struct {
	CTVID    *int64 `json:"ctv_id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Batches  int    `json:"batches"`
	Houses   int    `json:"houses"`
	Total    int64  `json:"total"`
}

// PaidLine: 1 khoản đã chi (hồ sơ hoặc thưởng/trừ) cho file Excel báo cáo.
type PaidLine struct {
	At       time.Time
	BatchID  int64
	Name     string
	Username string
	Bank     string
	Item     string
	Detail   string
	Amount   int64
	Ref      string
}

type PaymentRepository interface {
	// Owed: CTV có hồ sơ đã duyệt chưa trả (ctvID = 0: mọi CTV), kèm thưởng/trừ chưa trả.
	Owed(ctx context.Context, ctvID int64) ([]model.PayRow, error)
	CountReady(ctx context.Context, payMin int) (int, error)
	// Pay lập 1 phiếu chi gồm mọi hồ sơ đã duyệt + thưởng/trừ chưa trả của CTV, trong 1 giao dịch.
	Pay(ctx context.Context, in PayInput) (int64, int64, error)

	Batches(ctx context.Context, limit int) ([]model.Batch, error)
	TotalPaid(ctx context.Context) (int64, error)
	Batch(ctx context.Context, id int64) (*model.Batch, error)
	SetRef(ctx context.Context, id int64, ref string) error
	SetReceipt(ctx context.Context, id int64, name string) error
	// Cancel huỷ phiếu: hồ sơ về chờ thanh toán, thưởng/trừ về chưa trả. Trả id các hồ sơ.
	Cancel(ctx context.Context, id int64) ([]int64, error)
	Payouts(ctx context.Context, ctvID int64) ([]model.Payout, error)

	Adjustments(ctx context.Context, ctvID int64, pendingOnly bool) ([]model.Adjustment, error)
	BatchAdjustments(ctx context.Context, batchID, ctvID int64) ([]model.Adjustment, error)
	AddAdjustment(ctx context.Context, a *model.Adjustment) error
	Adjustment(ctx context.Context, id int64) (*model.Adjustment, error)
	DeleteAdjustment(ctx context.Context, id int64) error

	Periods(ctx context.Context, keySQL string, from, to time.Time) ([]Period, error)
	PerCTV(ctx context.Context, from, to time.Time) ([]PerCTV, error)
	PaidLines(ctx context.Context, from, to time.Time) ([]PaidLine, error)
}

type paymentRepository struct {
	db *pgxpool.Pool
}

func NewPaymentRepository(db *pgxpool.Pool) PaymentRepository {
	return &paymentRepository{db: db}
}

func (r *paymentRepository) Owed(ctx context.Context, ctvID int64) ([]model.PayRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.username, u.bank, COUNT(*), SUM(h.amount) + `+adjPendSQL+`
		FROM houses h JOIN users u ON u.id = h.ctv_id
		WHERE h.status = 'da_duyet' AND u.locked < 2 AND ($1 = 0 OR u.id = $1)
		GROUP BY u.id ORDER BY COUNT(*) DESC, 6 DESC`, ctvID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PayRow{}
	for rows.Next() {
		var p model.PayRow
		if err := rows.Scan(&p.CTVID, &p.Name, &p.Username, &p.Bank, &p.HouseCount, &p.Total); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *paymentRepository) CountReady(ctx context.Context, payMin int) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM houses h JOIN users u ON u.id = h.ctv_id
		WHERE h.status = 'da_duyet' AND u.locked < 2 GROUP BY h.ctv_id HAVING COUNT(*) >= $1) x`, payMin).Scan(&n)
	return n, err
}

func (r *paymentRepository) Pay(ctx context.Context, in PayInput) (int64, int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Khoá dòng CTV: 2 lần bấm cùng lúc không trả trùng
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, in.CTVID); err != nil {
		return 0, 0, err
	}
	type item struct {
		ID     int64
		Amount int64
	}
	load := func(q string) ([]item, error) {
		rows, err := tx.Query(ctx, q, in.CTVID)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowToStructByPos[item])
	}
	houses, err := load(`SELECT id, amount FROM houses WHERE ctv_id = $1 AND status = 'da_duyet' ORDER BY id FOR UPDATE`)
	if err != nil {
		return 0, 0, err
	}
	adjs, err := load(`SELECT id, amount FROM adjustments WHERE ctv_id = $1 AND batch_id IS NULL ORDER BY id FOR UPDATE`)
	if err != nil {
		return 0, 0, err
	}
	var total int64
	hids, aids := []int64{}, []int64{}
	for _, h := range houses {
		total += h.Amount
		hids = append(hids, h.ID)
	}
	for _, a := range adjs {
		total += a.Amount
		aids = append(aids, a.ID)
	}
	sorted := func(s []int64) []int64 { c := slices.Clone(s); slices.Sort(c); return c }
	if (in.HouseIDs != nil && !slices.Equal(sorted(in.HouseIDs), hids)) ||
		(in.AdjIDs != nil && !slices.Equal(sorted(in.AdjIDs), aids)) ||
		(in.SeenTotal != nil && *in.SeenTotal != total) {
		return 0, 0, ErrStale
	}
	if len(houses) < in.PayMin {
		return 0, 0, ErrNotEnough
	}
	if total <= 0 {
		return 0, 0, ErrZeroTotal
	}
	var bid int64
	if err := tx.QueryRow(ctx, `INSERT INTO batches (status, total, ref, created_by, ctv_id) VALUES ('da_chi', $1, $2, $3, $4)
		RETURNING id`, total, in.Ref, in.By, in.CTVID).Scan(&bid); err != nil {
		return 0, 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE houses SET status = 'da_chi', batch_id = $1 WHERE id = ANY($2)`, bid, hids); err != nil {
		return 0, 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adjustments SET batch_id = $1 WHERE id = ANY($2)`, bid, aids); err != nil {
		return 0, 0, err
	}
	note := pluralNote(len(hids), len(aids), total, in.Ref)
	if _, err := tx.Exec(ctx, `INSERT INTO house_logs (user_id, action, batch_id, note) VALUES ($1, 'Đã thanh toán', $2, $3)`,
		in.By, bid, note); err != nil {
		return 0, 0, err
	}
	return bid, total, tx.Commit(ctx)
}

func pluralNote(houses, adjs int, total int64, ref string) string {
	s := itoa64(int64(houses)) + " hồ sơ"
	if adjs > 0 {
		s += ", " + itoa64(int64(adjs)) + " khoản thưởng/trừ"
	}
	s += ", " + moneyStr(total)
	if ref != "" {
		s += ", mã giao dịch " + ref
	}
	return s
}

const batchSelect = `SELECT b.id, b.status, b.total, b.ref, b.receipt, b.ctv_id, COALESCE(u.name, ''), COALESCE(u.username, ''),
	COALESCE(u.bank, ''), b.created_by, COALESCE(k.name, ''), b.created_at,
	(SELECT COUNT(*) FROM houses WHERE batch_id = b.id)
	FROM batches b LEFT JOIN users u ON u.id = b.ctv_id LEFT JOIN users k ON k.id = b.created_by `

func scanBatch(row pgx.Row) (*model.Batch, error) {
	var b model.Batch
	err := row.Scan(&b.ID, &b.Status, &b.Total, &b.Ref, &b.Receipt, &b.CTVID, &b.CTVName, &b.CTVPhone, &b.CTVBank,
		&b.CreatedBy, &b.ByName, &b.CreatedAt, &b.HouseCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.HasReceipt = b.Receipt != ""
	if b.HasReceipt {
		b.ReceiptVersion = b.Receipt
		if i := len(b.Receipt) - 12; i > 0 { // "<id>-<8 hex>.jpg": dùng phần ngẫu nhiên làm phiên bản
			b.ReceiptVersion = b.Receipt[i : len(b.Receipt)-4]
		}
	}
	return &b, nil
}

func (r *paymentRepository) Batches(ctx context.Context, limit int) ([]model.Batch, error) {
	rows, err := r.db.Query(ctx, batchSelect+`ORDER BY b.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Batch{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *paymentRepository) TotalPaid(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COALESCE(SUM(total), 0) FROM batches WHERE status = 'da_chi'`).Scan(&n)
	return n, err
}

func (r *paymentRepository) Batch(ctx context.Context, id int64) (*model.Batch, error) {
	return scanBatch(r.db.QueryRow(ctx, batchSelect+`WHERE b.id = $1`, id))
}

func (r *paymentRepository) SetRef(ctx context.Context, id int64, ref string) error {
	tag, err := r.db.Exec(ctx, `UPDATE batches SET ref = $2 WHERE id = $1 AND status = 'da_chi'`, id, ref)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotPayable
	}
	return err
}

func (r *paymentRepository) SetReceipt(ctx context.Context, id int64, name string) error {
	_, err := r.db.Exec(ctx, `UPDATE batches SET receipt = $2 WHERE id = $1`, id, name)
	return err
}

func (r *paymentRepository) Cancel(ctx context.Context, id int64) ([]int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE batches SET status = 'huy' WHERE id = $1 AND status = 'da_chi'`, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotPayable
	}
	rows, err := tx.Query(ctx, `UPDATE houses SET status = 'da_duyet', batch_id = NULL WHERE batch_id = $1 RETURNING id`, id)
	if err != nil {
		return nil, err
	}
	hids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adjustments SET batch_id = NULL WHERE batch_id = $1`, id); err != nil {
		return nil, err
	}
	slices.Sort(hids)
	return hids, tx.Commit(ctx)
}

func (r *paymentRepository) Payouts(ctx context.Context, ctvID int64) ([]model.Payout, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.created_at, b.ref, COUNT(h.id),
			COALESCE(SUM(h.amount), 0) + COALESCE((SELECT SUM(a.amount) FROM adjustments a WHERE a.batch_id = b.id AND a.ctv_id = $1), 0)
		FROM batches b JOIN houses h ON h.batch_id = b.id
		WHERE h.ctv_id = $1 AND b.status = 'da_chi'
		GROUP BY b.id ORDER BY b.id DESC`, ctvID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Payout{}
	for rows.Next() {
		var p model.Payout
		if err := rows.Scan(&p.ID, &p.CreatedAt, &p.Ref, &p.HouseCount, &p.Total); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const adjSelect = `SELECT a.id, a.ctv_id, a.amount, a.note, a.batch_id, a.created_by, COALESCE(k.name, ''), a.created_at
	FROM adjustments a LEFT JOIN users k ON k.id = a.created_by `

func (r *paymentRepository) adjs(ctx context.Context, q string, args ...any) ([]model.Adjustment, error) {
	rows, err := r.db.Query(ctx, adjSelect+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Adjustment{}
	for rows.Next() {
		var a model.Adjustment
		if err := rows.Scan(&a.ID, &a.CTVID, &a.Amount, &a.Note, &a.BatchID, &a.CreatedBy, &a.ByName, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *paymentRepository) Adjustments(ctx context.Context, ctvID int64, pendingOnly bool) ([]model.Adjustment, error) {
	if pendingOnly {
		return r.adjs(ctx, `WHERE a.ctv_id = $1 AND a.batch_id IS NULL ORDER BY a.id DESC`, ctvID)
	}
	return r.adjs(ctx, `WHERE a.ctv_id = $1 ORDER BY a.id DESC`, ctvID)
}

func (r *paymentRepository) BatchAdjustments(ctx context.Context, batchID, ctvID int64) ([]model.Adjustment, error) {
	return r.adjs(ctx, `WHERE a.batch_id = $1 AND ($2 = 0 OR a.ctv_id = $2) ORDER BY a.id`, batchID, ctvID)
}

func (r *paymentRepository) AddAdjustment(ctx context.Context, a *model.Adjustment) error {
	return r.db.QueryRow(ctx, `INSERT INTO adjustments (ctv_id, amount, note, created_by) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`, a.CTVID, a.Amount, a.Note, a.CreatedBy).Scan(&a.ID, &a.CreatedAt)
}

func (r *paymentRepository) Adjustment(ctx context.Context, id int64) (*model.Adjustment, error) {
	list, err := r.adjs(ctx, `WHERE a.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

func (r *paymentRepository) DeleteAdjustment(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM adjustments WHERE id = $1 AND batch_id IS NULL`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Periods: keySQL là biểu thức nhóm đã kiểm (không lấy từ người dùng).
func (r *paymentRepository) Periods(ctx context.Context, keySQL string, from, to time.Time) ([]Period, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+keySQL+` AS k, COUNT(*), COUNT(DISTINCT b.ctv_id),
			COALESCE(SUM((SELECT COUNT(*) FROM houses WHERE batch_id = b.id)), 0), COALESCE(SUM(b.total), 0)
		FROM batches b WHERE b.status = 'da_chi' AND b.created_at >= $1 AND b.created_at < $2
		GROUP BY k ORDER BY k DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Period{}
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.Key, &p.Batches, &p.CTVs, &p.Houses, &p.Total); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *paymentRepository) PerCTV(ctx context.Context, from, to time.Time) ([]PerCTV, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.ctv_id, COALESCE(u.name, ''), COALESCE(u.username, ''), COUNT(*),
			COALESCE(SUM((SELECT COUNT(*) FROM houses WHERE batch_id = b.id)), 0), COALESCE(SUM(b.total), 0)
		FROM batches b LEFT JOIN users u ON u.id = b.ctv_id
		WHERE b.status = 'da_chi' AND b.created_at >= $1 AND b.created_at < $2
		GROUP BY b.ctv_id, u.name, u.username ORDER BY 6 DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PerCTV{}
	for rows.Next() {
		var p PerCTV
		if err := rows.Scan(&p.CTVID, &p.Name, &p.Username, &p.Batches, &p.Houses, &p.Total); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *paymentRepository) PaidLines(ctx context.Context, from, to time.Time) ([]PaidLine, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.created_at, b.id, u.name, u.username, u.bank, 'Hồ sơ #' || h.id, h.address, h.amount, b.ref
		FROM batches b JOIN houses h ON h.batch_id = b.id JOIN users u ON u.id = h.ctv_id
		WHERE b.status = 'da_chi' AND b.created_at >= $1 AND b.created_at < $2
		UNION ALL
		SELECT b.created_at, b.id, u.name, u.username, u.bank, 'Thưởng / trừ', a.note, a.amount, b.ref
		FROM batches b JOIN adjustments a ON a.batch_id = b.id JOIN users u ON u.id = a.ctv_id
		WHERE b.status = 'da_chi' AND b.created_at >= $1 AND b.created_at < $2
		ORDER BY 1, 2`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PaidLine{}
	for rows.Next() {
		var l PaidLine
		if err := rows.Scan(&l.At, &l.BatchID, &l.Name, &l.Username, &l.Bank, &l.Item, &l.Detail, &l.Amount, &l.Ref); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func itoa64(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// moneyStr "35.000đ" (trùng service.Money, để repository không phụ thuộc service).
func moneyStr(n int64) string {
	s := itoa64(n)
	neg := s[0] == '-'
	if neg {
		s = s[1:]
	}
	out := []byte{}
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out) + "đ"
	}
	return string(out) + "đ"
}
