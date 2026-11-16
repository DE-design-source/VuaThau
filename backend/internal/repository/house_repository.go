package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

// HouseFilter: điều kiện lọc danh sách hồ sơ. Trường rỗng / 0 là không lọc.
type HouseFilter struct {
	Statuses []string
	CTVID    int64
	BatchID  int64
	Q        string     // mã hồ sơ, địa chỉ, nhà thầu, tên / SĐT CTV
	From, To *time.Time // created_at trong [From, To)
}

// GeoPoint: toạ độ 1 hồ sơ (cảnh báo nhà trùng, bản đồ).
type GeoPoint struct {
	ID       int64
	Lat, Lng float64
}

type HouseRepository interface {
	Create(ctx context.Context, ctvID int64) (int64, error)
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*model.House, error)
	List(ctx context.Context, f HouseFilter, limit, offset int) ([]model.House, error)
	Count(ctx context.Context, f HouseFilter) (int, error)
	CountByStatus(ctx context.Context, f HouseFilter) (map[string]int, error)
	AmountByStatus(ctx context.Context) (map[string]int64, error)

	UpdateFields(ctx context.Context, id int64, f model.HouseFields) error
	FillAddressIfEmpty(ctx context.Context, id int64, addr string) error
	Submit(ctx context.Context, id int64, amount int64, lat, lng *float64, dup string, queueDrive bool) error
	SetStatus(ctx context.Context, id int64, from []string, to, note string) (bool, error)
	ApproveMany(ctx context.Context, ids []int64) ([]int64, error)
	SetAmount(ctx context.Context, id int64, amount int64) error
	NextPending(ctx context.Context, after int64) (*int64, int, error)
	Nearby(ctx context.Context, exclude int64) ([]GeoPoint, error)

	SetAIState(ctx context.Context, id int64, state string) error
	// ApplyAI điền các ô còn trống bằng giá trị AI đọc, ghi ai_state + ai_json.
	ApplyAI(ctx context.Context, id int64, got model.HouseFields, ok bool) error

	DriveQueue(ctx context.Context) ([]model.House, error)
	SetDrive(ctx context.Context, id int64, folder string) error

	CTVMoney(ctx context.Context, ctvID int64) (model.Money, error)
	CTVTodo(ctx context.Context, ctvID int64) (int, error)
	CTVStatusCounts(ctx context.Context, ctvID int64) (map[string]int, error)
}

type houseRepository struct {
	db *pgxpool.Pool
}

func NewHouseRepository(db *pgxpool.Pool) HouseRepository {
	return &houseRepository{db: db}
}

const houseCols = `h.id, h.ctv_id, u.name, u.username, u.bank, h.status, h.address, h.lat, h.lng, h.contractor_name,
	h.contractor_phone, h.floors, h.stage, h.note, h.dup, h.amount, h.ai_state, h.ai_json, h.batch_id, h.drive, h.created_at`

const houseFrom = ` FROM houses h JOIN users u ON u.id = h.ctv_id `

func scanHouse(row pgx.Row) (*model.House, error) {
	var h model.House
	err := row.Scan(&h.ID, &h.CTVID, &h.CTVName, &h.CTVPhone, &h.CTVBank, &h.Status, &h.Address, &h.Lat, &h.Lng,
		&h.ContractorName, &h.ContractorPhone, &h.Floors, &h.Stage, &h.Note, &h.Dup, &h.Amount, &h.AIState, &h.AIJSON,
		&h.BatchID, &h.Drive, &h.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &h, err
}

func (r *houseRepository) Create(ctx context.Context, ctvID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO houses (ctv_id) VALUES ($1) RETURNING id`, ctvID).Scan(&id)
	return id, err
}

func (r *houseRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM houses WHERE id = $1`, id)
	return err
}

func (r *houseRepository) Get(ctx context.Context, id int64) (*model.House, error) {
	return scanHouse(r.db.QueryRow(ctx, `SELECT `+houseCols+houseFrom+`WHERE h.id = $1`, id))
}

// where dựng điều kiện lọc; skipStatus: bỏ lọc trạng thái (để đếm số trên các chip).
func (f HouseFilter) where(skipStatus bool) (string, []any) {
	w, args := []string{"TRUE"}, []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		w = append(w, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if !skipStatus && len(f.Statuses) > 0 {
		add("h.status = ANY(?)", f.Statuses)
	}
	if f.CTVID > 0 {
		add("h.ctv_id = ?", f.CTVID)
	}
	if f.BatchID > 0 {
		add("h.batch_id = ?", f.BatchID)
	}
	if f.From != nil {
		add("h.created_at >= ?", *f.From)
	}
	if f.To != nil {
		add("h.created_at < ?", *f.To)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		digits := strings.TrimPrefix(spacesRE.ReplaceAllString(q, ""), "#")
		id := int64(-1)
		if n, err := strconv.ParseInt(digits, 10, 64); err == nil && len(digits) <= 9 {
			id = n
		}
		args = append(args, id, "%"+q+"%")
		ia, la := len(args)-1, len(args)
		cond := fmt.Sprintf("(h.id = $%d OR h.address ILIKE $%d OR h.contractor_name ILIKE $%d OR u.name ILIKE $%d", ia, la, la, la)
		if _, err := strconv.Atoi(digits); err == nil && len(digits) >= 3 { // chỉ so SĐT khi gõ từ 3 chữ số
			args = append(args, "%"+digits+"%")
			cond += fmt.Sprintf(" OR h.contractor_phone LIKE $%d OR u.username LIKE $%d", len(args), len(args))
		}
		w = append(w, cond+")")
	}
	return strings.Join(w, " AND "), args
}

func (r *houseRepository) List(ctx context.Context, f HouseFilter, limit, offset int) ([]model.House, error) {
	where, args := f.where(false)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `SELECT `+houseCols+houseFrom+`WHERE `+where+
		fmt.Sprintf(` ORDER BY h.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.House, 0)
	for rows.Next() {
		h, err := scanHouse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

func (r *houseRepository) Count(ctx context.Context, f HouseFilter) (int, error) {
	where, args := f.where(false)
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*)`+houseFrom+`WHERE `+where, args...).Scan(&n)
	return n, err
}

func (r *houseRepository) CountByStatus(ctx context.Context, f HouseFilter) (map[string]int, error) {
	where, args := f.where(true)
	rows, err := r.db.Query(ctx, `SELECT h.status, COUNT(*)`+houseFrom+`WHERE `+where+` GROUP BY h.status`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

func (r *houseRepository) AmountByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT status, COALESCE(SUM(amount), 0) FROM houses GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var s string
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

func (r *houseRepository) exec(ctx context.Context, q string, args ...any) error {
	tag, err := r.db.Exec(ctx, q, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *houseRepository) UpdateFields(ctx context.Context, id int64, f model.HouseFields) error {
	return r.exec(ctx, `UPDATE houses SET address = $2, floors = $3, stage = $4, contractor_name = $5, contractor_phone = $6
		WHERE id = $1`, id, f.Address, f.Floors, f.Stage, f.ContractorName, f.ContractorPhone)
}

func (r *houseRepository) FillAddressIfEmpty(ctx context.Context, id int64, addr string) error {
	_, err := r.db.Exec(ctx, `UPDATE houses SET address = $2 WHERE id = $1 AND address = ''`, id, addr)
	return err
}

func (r *houseRepository) Submit(ctx context.Context, id int64, amount int64, lat, lng *float64, dup string, queueDrive bool) error {
	drive := "drive"
	if queueDrive {
		drive = "'cho'"
	}
	return r.exec(ctx, `UPDATE houses SET status = 'cho_duyet', note = '', amount = $2, lat = $3, lng = $4, dup = $5, drive = `+drive+`
		WHERE id = $1 AND status IN ('nhap', 'tra_lai')`, id, amount, lat, lng, dup)
}

// SetStatus đổi trạng thái nếu đang ở 1 trong from. Trả false nếu không ở trạng thái đó.
func (r *houseRepository) SetStatus(ctx context.Context, id int64, from []string, to, note string) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE houses SET status = $3, note = $4 WHERE id = $1 AND status = ANY($2)`, id, from, to, note)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *houseRepository) ApproveMany(ctx context.Context, ids []int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `UPDATE houses SET status = 'da_duyet', note = '' WHERE status = 'cho_duyet' AND id = ANY($1)
		RETURNING id`, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

func (r *houseRepository) SetAmount(ctx context.Context, id int64, amount int64) error {
	return r.exec(ctx, `UPDATE houses SET amount = $2 WHERE id = $1`, id, amount)
}

// NextPending: hồ sơ chờ duyệt kế tiếp sau id (vòng lại từ đầu) và số hồ sơ chờ duyệt còn lại.
func (r *houseRepository) NextPending(ctx context.Context, after int64) (*int64, int, error) {
	var left int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM houses WHERE status = 'cho_duyet'`).Scan(&left); err != nil {
		return nil, 0, err
	}
	var id int64
	err := r.db.QueryRow(ctx, `SELECT id FROM houses WHERE status = 'cho_duyet' ORDER BY id <= $1, id LIMIT 1`, after).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, left, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return &id, left, nil
}

func (r *houseRepository) Nearby(ctx context.Context, exclude int64) ([]GeoPoint, error) {
	rows, err := r.db.Query(ctx, `SELECT id, lat, lng FROM houses WHERE id <> $1 AND lat IS NOT NULL AND lng IS NOT NULL
		AND status <> 'tu_choi'`, exclude)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GeoPoint{}
	for rows.Next() {
		var p GeoPoint
		if err := rows.Scan(&p.ID, &p.Lat, &p.Lng); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *houseRepository) SetAIState(ctx context.Context, id int64, state string) error {
	_, err := r.db.Exec(ctx, `UPDATE houses SET ai_state = $2 WHERE id = $1`, id, state)
	return err
}

func (r *houseRepository) ApplyAI(ctx context.Context, id int64, got model.HouseFields, ok bool) error {
	state := model.AIDone
	var js any
	if !ok {
		state = model.AIFailed
	} else {
		floors := 0
		if got.Floors != nil {
			floors = *got.Floors
		}
		js = map[string]any{"address": got.Address, "contractor_name": got.ContractorName,
			"contractor_phone": got.ContractorPhone, "floors": floors, "stage": got.Stage}
	}
	_, err := r.db.Exec(ctx, `UPDATE houses SET
		address = CASE WHEN address = '' THEN $2 ELSE address END,
		contractor_name = CASE WHEN contractor_name = '' THEN $3 ELSE contractor_name END,
		contractor_phone = CASE WHEN contractor_phone = '' THEN $4 ELSE contractor_phone END,
		floors = CASE WHEN COALESCE(floors, 0) = 0 THEN $5 ELSE floors END,
		stage = CASE WHEN stage = '' THEN $6 ELSE stage END,
		ai_state = $7, ai_json = $8
		WHERE id = $1`, id, got.Address, got.ContractorName, got.ContractorPhone, got.Floors, got.Stage, state, js)
	return err
}

func (r *houseRepository) DriveQueue(ctx context.Context) ([]model.House, error) {
	rows, err := r.db.Query(ctx, `SELECT `+houseCols+houseFrom+`WHERE h.drive = 'cho' ORDER BY h.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.House{}
	for rows.Next() {
		h, err := scanHouse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

func (r *houseRepository) SetDrive(ctx context.Context, id int64, folder string) error {
	_, err := r.db.Exec(ctx, `UPDATE houses SET drive = $2 WHERE id = $1 AND drive = 'cho'`, id, folder)
	return err
}

func (r *houseRepository) CTVMoney(ctx context.Context, ctvID int64) (model.Money, error) {
	var m model.Money
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount) FILTER (WHERE status = 'da_chi'), 0),
			COALESCE(SUM(amount) FILTER (WHERE status = 'da_duyet'), 0),
			COUNT(*) FILTER (WHERE status = 'da_duyet'),
			COALESCE(SUM(amount) FILTER (WHERE status = 'cho_duyet'), 0)
		FROM houses WHERE ctv_id = $1`, ctvID).Scan(&m.Paid, &m.Owed, &m.Approved, &m.Reviewing)
	if err != nil {
		return m, err
	}
	var pend, paid int64
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(a.amount) FILTER (WHERE a.batch_id IS NULL), 0),
			COALESCE(SUM(a.amount) FILTER (WHERE b.status = 'da_chi'), 0)
		FROM adjustments a LEFT JOIN batches b ON b.id = a.batch_id WHERE a.ctv_id = $1`, ctvID).Scan(&pend, &paid)
	m.Owed += pend
	m.Paid += paid
	return m, err
}

func (r *houseRepository) CTVTodo(ctx context.Context, ctvID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM houses WHERE ctv_id = $1 AND status IN ('nhap', 'tra_lai')`, ctvID).Scan(&n)
	return n, err
}

func (r *houseRepository) CTVStatusCounts(ctx context.Context, ctvID int64) (map[string]int, error) {
	return r.CountByStatus(ctx, HouseFilter{CTVID: ctvID})
}
