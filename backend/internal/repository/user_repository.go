package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

// CTVRow: 1 CTV trong danh sách quản lý, kèm số liệu hồ sơ và tiền.
type CTVRow struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Locked    int        `json:"locked"`
	CreatedAt time.Time  `json:"created_at"`
	Houses    int        `json:"houses"`
	Waiting   int        `json:"waiting"`
	Approved  int        `json:"approved"`
	Paid      int64      `json:"paid"`
	Owed      int64      `json:"owed"`
	LastAt    *time.Time `json:"last_at"`
	Bank      string     `json:"bank"`        // "Ngân hàng · Số TK · CHỦ TK" (cột Số tài khoản)
	Region    string     `json:"region"`      // mã tỉnh: admin chọn, không thì tỉnh CTV chụp nhiều nhất ('' = chưa rõ)
	RegionAut bool       `json:"region_auto"` // Region là tự tính (admin chưa chọn)
	Kind      string     `json:"kind"`        // chức năng / loại CTV
	Note      string     `json:"note"`        // ghi chú nội bộ của admin
}

// CTVFilter: bộ lọc danh sách CTV. Region: mã tỉnh, "-" = chưa rõ khu vực. Bank: "co" | "chua".
type CTVFilter struct {
	Q, Region, Kind, Bank string
}

// CTVProfile: khu vực, chức năng, ghi chú của 1 CTV (admin sửa). RegionAuto: tỉnh tự tính (gợi ý khi chưa chọn).
type CTVProfile struct {
	Region     string `json:"region"`
	RegionAuto string `json:"region_auto"`
	Kind       string `json:"kind"`
	Note       string `json:"note"`
}

type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	ByInboxContact(ctx context.Context, contact string) (*model.User, error)
	SetInboxContact(ctx context.Context, id int64, contact string) error

	UpdateBank(ctx context.Context, id int64, bank string) error
	// UpdatePassword đổi mật khẩu và tăng token_version (token cũ hết hiệu lực).
	UpdatePassword(ctx context.Context, id int64, hash string) error
	// SetLocked khoá / mở khoá và tăng token_version.
	SetLocked(ctx context.Context, id int64, locked int) error
	// Anonymize: CTV tự xoá tài khoản. Xoá tên, SĐT, email, Google, ngân hàng; hồ sơ, phiếu chi giữ lại.
	Anonymize(ctx context.Context, id int64, hash string) error

	ListStaff(ctx context.Context) ([]model.User, error)
	ListCTVs(ctx context.Context, f CTVFilter, limit, offset int) ([]CTVRow, int, error)
	// CTVFacets: số CTV theo khu vực (mã tỉnh, '' = chưa rõ) và theo chức năng, cho ô lọc.
	CTVFacets(ctx context.Context) (regions, kinds map[string]int, err error)
	Profile(ctx context.Context, id int64) (*CTVProfile, error)
	SetProfile(ctx context.Context, id int64, region, kind, note string) error
	CountCTVs(ctx context.Context) (int, error)
	CountNewCTVs(ctx context.Context, from, to time.Time) (int, error)
}

type userRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return &userRepository{db: db}
}

const userCols = `id, username, name, role, COALESCE(email, ''), bank, locked, COALESCE(google_sub, ''), token_version,
	password_hash, created_at, updated_at`

func scanUser(row pgx.Row) (*model.User, error) {
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Email, &u.Bank, &u.Locked, &u.GoogleSub, &u.TokenVersion,
		&u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

// ByInboxContact: CTV đã ghép với liên hệ Messenger (contacts.id bên dezon-inbox).
func (r *userRepository) ByInboxContact(ctx context.Context, contact string) (*model.User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE inbox_contact = $1`, contact))
}

func (r *userRepository) SetInboxContact(ctx context.Context, id int64, contact string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET inbox_contact = $2 WHERE id = $1`, id, contact)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate
	}
	return err
}

func (r *userRepository) many(ctx context.Context, q string, args ...any) ([]model.User, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]model.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func (r *userRepository) exec(ctx context.Context, q string, args ...any) error {
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *userRepository) UpdateBank(ctx context.Context, id int64, bank string) error {
	return r.exec(ctx, `UPDATE users SET bank = $2 WHERE id = $1`, id, bank)
}

func (r *userRepository) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return r.exec(ctx, `UPDATE users SET password_hash = $2, token_version = token_version + 1 WHERE id = $1`, id, hash)
}

func (r *userRepository) SetLocked(ctx context.Context, id int64, locked int) error {
	return r.exec(ctx, `UPDATE users SET locked = $2, token_version = token_version + 1 WHERE id = $1`, id, locked)
}

func (r *userRepository) Anonymize(ctx context.Context, id int64, hash string) error {
	return r.exec(ctx, `
		UPDATE users SET name = $2, username = $3, email = NULL, google_sub = NULL, bank = '', password_hash = $4,
			locked = $5, token_version = token_version + 1
		WHERE id = $1`,
		id, fmt.Sprintf("CTV đã xoá #%d", id), fmt.Sprintf("xoa-%d", id), hash, model.LockedDeleted)
}

func (r *userRepository) ListStaff(ctx context.Context) ([]model.User, error) {
	return r.many(ctx, `SELECT `+userCols+` FROM users WHERE role IN ('admin', 'ketoan', 'sale') ORDER BY role, name`)
}

// Tiền thưởng/trừ: đã trả (nằm trong phiếu chi chưa huỷ) và chờ trả (chưa vào phiếu nào).
const (
	adjPaidSQL = `COALESCE((SELECT SUM(a.amount) FROM adjustments a JOIN batches b ON b.id = a.batch_id
		WHERE a.ctv_id = u.id AND b.status = 'da_chi'), 0)`
	adjPendSQL = `COALESCE((SELECT SUM(a.amount) FROM adjustments a WHERE a.ctv_id = u.id AND a.batch_id IS NULL), 0)`
)

// autoRegionSQL: tỉnh CTV (cột ctv) chụp nhiều công trình nhất (hồ sơ đã gửi, không tính bị từ chối); hoà thì tỉnh mới hơn.
func autoRegionSQL(ctv string) string {
	return `(SELECT h.province FROM houses h WHERE h.ctv_id = ` + ctv + ` AND COALESCE(h.province, '') <> ''
		AND h.status NOT IN ('nhap', 'tu_choi') GROUP BY h.province ORDER BY COUNT(*) DESC, MAX(h.id) DESC LIMIT 1)`
}

// ctvCTE: bảng tạm c = CTV kèm khu vực đang dùng (reg) và cờ admin đã chọn (manual).
var ctvCTE = `WITH c AS (
	SELECT u.id, u.username, u.name, COALESCE(u.email, '') AS email, u.locked, u.created_at, u.bank, u.kind, u.note,
		u.region <> '' AS manual, COALESCE(NULLIF(u.region, ''), ` + autoRegionSQL("u.id") + `, '') AS reg
	FROM users u WHERE u.role = 'ctv') `

func (r *userRepository) ListCTVs(ctx context.Context, f CTVFilter, limit, offset int) ([]CTVRow, int, error) {
	const where = `($1 = '' OR c.name ILIKE '%' || $1 || '%' OR c.email ILIKE '%' || $1 || '%' OR c.note ILIKE '%' || $1 || '%'
		OR c.username LIKE '%' || $2 || '%')
		AND ($3 = '' OR c.reg = $3 OR ($3 = '-' AND c.reg = ''))
		AND ($4 = '' OR c.kind = $4)
		AND ($5 = '' OR ($5 = 'co') = (c.bank <> ''))`
	q := f.Q
	digits := spacesRE.ReplaceAllString(q, "")
	if digits == "" {
		digits = q
	}
	args := []any{q, digits, f.Region, f.Kind, f.Bank}
	var found int
	if err := r.db.QueryRow(ctx, ctvCTE+`SELECT COUNT(*) FROM c WHERE `+where, args...).Scan(&found); err != nil {
		return nil, 0, err
	}
	paid := strings.ReplaceAll(adjPaidSQL, "u.id", "c.id")
	pend := strings.ReplaceAll(adjPendSQL, "u.id", "c.id")
	rows, err := r.db.Query(ctx, ctvCTE+`
		SELECT c.id, c.username, c.name, c.email, c.locked, c.created_at, COUNT(h.id),
			COUNT(h.id) FILTER (WHERE h.status = 'cho_duyet'), COUNT(h.id) FILTER (WHERE h.status = 'da_duyet'),
			COALESCE(SUM(h.amount) FILTER (WHERE h.status = 'da_chi'), 0) + `+paid+`,
			COALESCE(SUM(h.amount) FILTER (WHERE h.status = 'da_duyet'), 0) + `+pend+`,
			MAX(h.created_at), c.bank, c.reg, NOT c.manual AND c.reg <> '', c.kind, c.note
		FROM c LEFT JOIN houses h ON h.ctv_id = c.id
		WHERE `+where+`
		GROUP BY c.id, c.username, c.name, c.email, c.locked, c.created_at, c.bank, c.reg, c.manual, c.kind, c.note
		ORDER BY COALESCE(MAX(h.id), 0) DESC, c.id DESC
		LIMIT $6 OFFSET $7`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]CTVRow, 0)
	for rows.Next() {
		var c CTVRow
		if err := rows.Scan(&c.ID, &c.Username, &c.Name, &c.Email, &c.Locked, &c.CreatedAt, &c.Houses, &c.Waiting,
			&c.Approved, &c.Paid, &c.Owed, &c.LastAt, &c.Bank, &c.Region, &c.RegionAut, &c.Kind, &c.Note); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, found, rows.Err()
}

func (r *userRepository) CTVFacets(ctx context.Context) (map[string]int, map[string]int, error) {
	count := func(q string) (map[string]int, error) {
		rows, err := r.db.Query(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		m := map[string]int{}
		for rows.Next() {
			var k string
			var n int
			if err := rows.Scan(&k, &n); err != nil {
				return nil, err
			}
			m[k] = n
		}
		return m, rows.Err()
	}
	regions, err := count(ctvCTE + `SELECT reg, COUNT(*) FROM c GROUP BY reg`)
	if err != nil {
		return nil, nil, err
	}
	kinds, err := count(`SELECT kind, COUNT(*) FROM users WHERE role = 'ctv' GROUP BY kind`)
	return regions, kinds, err
}

func (r *userRepository) Profile(ctx context.Context, id int64) (*CTVProfile, error) {
	var p CTVProfile
	err := r.db.QueryRow(ctx, `SELECT u.region, COALESCE(`+autoRegionSQL("u.id")+`, ''), u.kind, u.note
		FROM users u WHERE u.id = $1`, id).Scan(&p.Region, &p.RegionAuto, &p.Kind, &p.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *userRepository) SetProfile(ctx context.Context, id int64, region, kind, note string) error {
	return r.exec(ctx, `UPDATE users SET region = $2, kind = $3, note = $4 WHERE id = $1 AND role = 'ctv'`, id, region, kind, note)
}

func (r *userRepository) CountCTVs(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'ctv' AND locked < 2`).Scan(&n)
	return n, err
}

func (r *userRepository) CountNewCTVs(ctx context.Context, from, to time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'ctv' AND created_at >= $1 AND created_at < $2`,
		from, to).Scan(&n)
	return n, err
}
