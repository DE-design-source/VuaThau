package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Nhà thầu trên app: hồ sơ công ty, dự án, nhật ký thi công, ảnh / video nhật ký.

type NTProfile struct {
	UserID      int64    `json:"user_id"`
	Company     string   `json:"company"`
	Specialties []string `json:"specialties"`
	Regions     string   `json:"regions"`
	Years       int      `json:"years"`
	Verified    bool     `json:"verified"`
	Rating      float64  `json:"rating"`
	Reviews     int      `json:"reviews"`
	Views       int      `json:"views"`
}

type NTProject struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	Address    string     `json:"address"`
	Province   string     `json:"province"` // tỉnh / thành (form Đăng dự án)
	Ward       string     `json:"ward"`     // phường / xã / đặc khu
	Street     string     `json:"street"`   // số nhà, tên đường
	Kind       string     `json:"kind"`
	Status     string     `json:"status"` // dang_thi_cong | hoan_thanh
	Progress   int        `json:"progress"`
	StartedOn  time.Time  `json:"started_on"`
	FinishedOn *time.Time `json:"finished_on"`
	Lat        *float64   `json:"lat"`
	Lng        *float64   `json:"lng"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	// thống kê (danh sách / chi tiết)
	Logs    int        `json:"logs"`
	Photos  int        `json:"photos"`
	Videos  int        `json:"videos"`
	LastLog *time.Time `json:"last_log"`
	Cover   *int64     `json:"cover"` // ảnh bìa: ảnh dự án đầu tiên, không có thì ảnh nhật ký mới nhất
	// HouseID: dự án tạo từ công trình CTV đã chụp (bảng hiệu có SĐT nhà thầu); nil: nhà thầu tự đăng
	HouseID    *int64 `json:"house_id"`
	HousePhoto *int64 `json:"-"`                   // ảnh đầu của hồ sơ CTV (ảnh bìa khi dự án chưa có ảnh)
	CoverURL   string `json:"cover_url,omitempty"` // link công khai ảnh HousePhoto (service điền)
}

type NTLog struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Note      string    `json:"note"`
	TakenAt   time.Time `json:"taken_at"`
	Lat       *float64  `json:"lat"`
	Lng       *float64  `json:"lng"`
	Address   string    `json:"address"`
	Media     []NTMedia `json:"media"`
}

type NTMedia struct {
	ID        int64  `json:"id"`
	LogID     int64  `json:"log_id"` // 0: ảnh dự án (form Đăng dự án), không thuộc nhật ký nào
	ProjectID int64  `json:"project_id"`
	Kind      string `json:"kind"` // photo | video
	SHA256    string `json:"-"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	Src       string `json:"src,omitempty"` // video: link có chữ ký
}

type NTRepository interface {
	Profile(ctx context.Context, uid int64) (*NTProfile, error)
	UpsertProfile(ctx context.Context, p *NTProfile) error
	Projects(ctx context.Context, uid int64, status string) ([]NTProject, error)
	Project(ctx context.Context, id int64) (*NTProject, error)
	CreateProject(ctx context.Context, p *NTProject) error
	UpdateProject(ctx context.Context, p *NTProject) error
	DeleteProject(ctx context.Context, id int64) error
	Logs(ctx context.Context, projectID int64) ([]NTLog, error)
	CreateLog(ctx context.Context, l *NTLog) error
	SetLogNote(ctx context.Context, id int64, note string) error
	DeleteLog(ctx context.Context, id int64) error
	AddMedia(ctx context.Context, m *NTMedia) error
	Media(ctx context.Context, id int64) (*NTMedia, error)
	ProjectByHouse(ctx context.Context, uid, hid int64) (*NTProject, error)
	// SetLarkRecord: tài khoản gắn với dòng form đăng ký nhà thầu trên Lark (đồng bộ xác minh / bỏ xác minh)
	SetLarkRecord(ctx context.Context, uid int64, record string) error
	LarkRecord(ctx context.Context, uid int64) (string, error)
	// SwapLock: đổi trạng thái khoá nhà thầu from -> to (chỉ khi đang đúng from: không đè khoá của admin); đăng xuất mọi máy.
	SwapLock(ctx context.Context, uid int64, from, to int) (bool, error)
	// UserPhone: SĐT đăng nhập của nhà thầu (nút Gọi / Zalo trên báo cáo gửi chủ nhà)
	UserPhone(ctx context.Context, uid int64) (string, error)
	// ProjectPhotos: ảnh dự án (không thuộc nhật ký), ảnh bìa trước
	ProjectPhotos(ctx context.Context, projectID int64) ([]NTMedia, error)
	// Log: 1 nhật ký kèm ảnh / video (báo cáo gửi chủ nhà)
	Log(ctx context.Context, id int64) (*NTLog, error)
	// Contractors: mọi tài khoản nhà thầu (trang admin)
	Contractors(ctx context.Context) ([]NTContractor, error)
}

// NTContractor: 1 nhà thầu trong trang admin.
type NTContractor struct {
	ID        int64      `json:"id"`
	Phone     string     `json:"phone"`
	Name      string     `json:"name"`
	Company   string     `json:"company"`
	Locked    int        `json:"locked"`
	Projects  int        `json:"projects"`
	Logs      int        `json:"logs"`
	LastLog   *time.Time `json:"last_log"`
	CreatedAt time.Time  `json:"created_at"`
}

type ntRepository struct{ db *pgxpool.Pool }

func NewNTRepository(db *pgxpool.Pool) NTRepository { return &ntRepository{db: db} }

func (r *ntRepository) Profile(ctx context.Context, uid int64) (*NTProfile, error) {
	p := &NTProfile{UserID: uid, Specialties: []string{}}
	err := r.db.QueryRow(ctx, `SELECT company, specialties, regions, years, verified, rating::float8, reviews, views FROM nt_profiles WHERE user_id = $1`, uid).
		Scan(&p.Company, &p.Specialties, &p.Regions, &p.Years, &p.Verified, &p.Rating, &p.Reviews, &p.Views)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	return p, err
}

func (r *ntRepository) UpsertProfile(ctx context.Context, p *NTProfile) error {
	if p.Specialties == nil {
		p.Specialties = []string{}
	}
	_, err := r.db.Exec(ctx, `INSERT INTO nt_profiles (user_id, company, specialties, regions, years, verified, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (user_id) DO UPDATE SET company = EXCLUDED.company, specialties = EXCLUDED.specialties, regions = EXCLUDED.regions,
			years = EXCLUDED.years, verified = EXCLUDED.verified, updated_at = NOW()`,
		p.UserID, p.Company, p.Specialties, p.Regions, p.Years, p.Verified)
	return err
}

const ntProjectSelect = `SELECT p.id, p.user_id, p.name, p.address, p.province, p.ward, p.street, p.kind, p.status, p.progress, p.started_on, p.finished_on, p.lat, p.lng,
	p.created_at, p.updated_at,
	(SELECT COUNT(*) FROM nt_logs l WHERE l.project_id = p.id),
	(SELECT COUNT(*) FROM nt_media m WHERE m.project_id = p.id AND m.kind = 'photo'),
	(SELECT COUNT(*) FROM nt_media m WHERE m.project_id = p.id AND m.kind = 'video'),
	(SELECT MAX(l.taken_at) FROM nt_logs l WHERE l.project_id = p.id),
	(SELECT m.id FROM nt_media m LEFT JOIN nt_logs l ON l.id = m.log_id WHERE m.project_id = p.id AND m.kind = 'photo'
		ORDER BY (m.log_id IS NULL) DESC, CASE WHEN m.log_id IS NULL THEN m.id END ASC, l.taken_at DESC, m.id DESC LIMIT 1),
	p.house_id,
	(SELECT ph.id FROM photos ph WHERE ph.house_id = p.house_id ORDER BY ph.id LIMIT 1)
	FROM nt_projects p`

func scanNTProject(row pgx.Row) (*NTProject, error) {
	var p NTProject
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Address, &p.Province, &p.Ward, &p.Street, &p.Kind, &p.Status, &p.Progress, &p.StartedOn, &p.FinishedOn, &p.Lat, &p.Lng,
		&p.CreatedAt, &p.UpdatedAt, &p.Logs, &p.Photos, &p.Videos, &p.LastLog, &p.Cover, &p.HouseID, &p.HousePhoto)
	return &p, err
}

// ProjectByHouse: dự án của nhà thầu gắn với công trình CTV đã chụp (chưa có: ErrNotFound).
func (r *ntRepository) ProjectByHouse(ctx context.Context, uid, hid int64) (*NTProject, error) {
	p, err := scanNTProject(r.db.QueryRow(ctx, ntProjectSelect+` WHERE p.user_id = $1 AND p.house_id = $2 AND p.deleted_at IS NULL`, uid, hid))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (r *ntRepository) Projects(ctx context.Context, uid int64, status string) ([]NTProject, error) {
	rows, err := r.db.Query(ctx, ntProjectSelect+` WHERE p.user_id = $1 AND p.deleted_at IS NULL AND ($2 = '' OR p.status = $2)
		ORDER BY (p.status = 'dang_thi_cong') DESC, p.updated_at DESC, p.id DESC`, uid, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NTProject{}
	for rows.Next() {
		p, err := scanNTProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *ntRepository) Project(ctx context.Context, id int64) (*NTProject, error) {
	p, err := scanNTProject(r.db.QueryRow(ctx, ntProjectSelect+` WHERE p.id = $1 AND p.deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (r *ntRepository) CreateProject(ctx context.Context, p *NTProject) error {
	err := r.db.QueryRow(ctx, `INSERT INTO nt_projects (user_id, name, address, province, ward, street, kind, lat, lng, house_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, status, progress, started_on, created_at, updated_at`, p.UserID, p.Name, p.Address, p.Province, p.Ward, p.Street, p.Kind, p.Lat, p.Lng, p.HouseID).
		Scan(&p.ID, &p.Status, &p.Progress, &p.StartedOn, &p.CreatedAt, &p.UpdatedAt)
	if pgErrCode(err) == pgUniqueViolation {
		return ErrDuplicate // cùng công trình CTV đã có dự án (bấm 2 lần cùng lúc)
	}
	return err
}

func (r *ntRepository) UpdateProject(ctx context.Context, p *NTProject) error {
	_, err := r.db.Exec(ctx, `UPDATE nt_projects SET name = $2, address = $3, kind = $4, status = $5, progress = $6, finished_on = $7,
		lat = COALESCE($8, lat), lng = COALESCE($9, lng), province = $10, ward = $11, street = $12, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		p.ID, p.Name, p.Address, p.Kind, p.Status, p.Progress, p.FinishedOn, p.Lat, p.Lng, p.Province, p.Ward, p.Street)
	return err
}

func (r *ntRepository) DeleteProject(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `UPDATE nt_projects SET deleted_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *ntRepository) Logs(ctx context.Context, projectID int64) ([]NTLog, error) {
	rows, err := r.db.Query(ctx, `SELECT id, project_id, note, taken_at, lat, lng, address FROM nt_logs WHERE project_id = $1 ORDER BY taken_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	out := []NTLog{}
	idx := map[int64]int{}
	for rows.Next() {
		var l NTLog
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.Note, &l.TakenAt, &l.Lat, &l.Lng, &l.Address); err != nil {
			rows.Close()
			return nil, err
		}
		l.Media = []NTMedia{}
		idx[l.ID] = len(out)
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mr, err := r.db.Query(ctx, `SELECT id, log_id, project_id, kind, sha256, mime, size FROM nt_media WHERE project_id = $1 AND log_id IS NOT NULL ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer mr.Close()
	for mr.Next() {
		var m NTMedia
		if err := mr.Scan(&m.ID, &m.LogID, &m.ProjectID, &m.Kind, &m.SHA256, &m.Mime, &m.Size); err != nil {
			return nil, err
		}
		if i, ok := idx[m.LogID]; ok {
			out[i].Media = append(out[i].Media, m)
		}
	}
	return out, mr.Err()
}

func (r *ntRepository) CreateLog(ctx context.Context, l *NTLog) error {
	if err := r.db.QueryRow(ctx, `INSERT INTO nt_logs (project_id, note, taken_at, lat, lng, address) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		l.ProjectID, l.Note, l.TakenAt, l.Lat, l.Lng, l.Address).Scan(&l.ID); err != nil {
		return err
	}
	// vị trí dự án: lấy theo nhật ký đầu tiên có GPS
	_, err := r.db.Exec(ctx, `UPDATE nt_projects SET lat = COALESCE(lat, $2), lng = COALESCE(lng, $3), updated_at = NOW() WHERE id = $1`, l.ProjectID, l.Lat, l.Lng)
	return err
}

func (r *ntRepository) SetLogNote(ctx context.Context, id int64, note string) error {
	_, err := r.db.Exec(ctx, `UPDATE nt_logs SET note = $2 WHERE id = $1`, id, note)
	return err
}

func (r *ntRepository) DeleteLog(ctx context.Context, id int64) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM nt_media WHERE log_id = $1`, id); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `DELETE FROM nt_logs WHERE id = $1`, id)
	return err
}

func (r *ntRepository) AddMedia(ctx context.Context, m *NTMedia) error {
	err := r.db.QueryRow(ctx, `INSERT INTO nt_media (log_id, project_id, kind, sha256, mime, size) VALUES (NULLIF($1, 0), $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, sha256) DO NOTHING RETURNING id`, m.LogID, m.ProjectID, m.Kind, m.SHA256, m.Mime, m.Size).Scan(&m.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDuplicate
	}
	return err
}

func (r *ntRepository) Media(ctx context.Context, id int64) (*NTMedia, error) {
	var m NTMedia
	err := r.db.QueryRow(ctx, `SELECT m.id, COALESCE(m.log_id, 0), m.project_id, m.kind, m.sha256, m.mime, m.size FROM nt_media m
		JOIN nt_projects p ON p.id = m.project_id AND p.deleted_at IS NULL WHERE m.id = $1`, id).
		Scan(&m.ID, &m.LogID, &m.ProjectID, &m.Kind, &m.SHA256, &m.Mime, &m.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

func (r *ntRepository) ProjectPhotos(ctx context.Context, projectID int64) ([]NTMedia, error) {
	rows, err := r.db.Query(ctx, `SELECT id, project_id, kind, sha256, mime, size FROM nt_media
		WHERE project_id = $1 AND log_id IS NULL AND kind = 'photo' ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NTMedia{}
	for rows.Next() {
		var m NTMedia
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Kind, &m.SHA256, &m.Mime, &m.Size); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ntRepository) Log(ctx context.Context, id int64) (*NTLog, error) {
	l := NTLog{Media: []NTMedia{}}
	err := r.db.QueryRow(ctx, `SELECT l.id, l.project_id, l.note, l.taken_at, l.lat, l.lng, l.address FROM nt_logs l
		JOIN nt_projects p ON p.id = l.project_id AND p.deleted_at IS NULL WHERE l.id = $1`, id).
		Scan(&l.ID, &l.ProjectID, &l.Note, &l.TakenAt, &l.Lat, &l.Lng, &l.Address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `SELECT id, log_id, project_id, kind, sha256, mime, size FROM nt_media WHERE log_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m NTMedia
		if err := rows.Scan(&m.ID, &m.LogID, &m.ProjectID, &m.Kind, &m.SHA256, &m.Mime, &m.Size); err != nil {
			return nil, err
		}
		l.Media = append(l.Media, m)
	}
	return &l, rows.Err()
}

func (r *ntRepository) Contractors(ctx context.Context) ([]NTContractor, error) {
	rows, err := r.db.Query(ctx, `SELECT u.id, u.username, u.name, COALESCE(pf.company, ''), u.locked, u.created_at,
			(SELECT COUNT(*) FROM nt_projects p WHERE p.user_id = u.id AND p.deleted_at IS NULL),
			(SELECT COUNT(*) FROM nt_logs l JOIN nt_projects p ON p.id = l.project_id WHERE p.user_id = u.id AND p.deleted_at IS NULL),
			(SELECT MAX(l.taken_at) FROM nt_logs l JOIN nt_projects p ON p.id = l.project_id WHERE p.user_id = u.id AND p.deleted_at IS NULL)
		FROM users u LEFT JOIN nt_profiles pf ON pf.user_id = u.id
		WHERE u.role = 'nhathau' AND u.locked <> 2 ORDER BY u.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NTContractor{}
	for rows.Next() {
		var c NTContractor
		if err := rows.Scan(&c.ID, &c.Phone, &c.Name, &c.Company, &c.Locked, &c.CreatedAt, &c.Projects, &c.Logs, &c.LastLog); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ntRepository) SetLarkRecord(ctx context.Context, uid int64, record string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO nt_profiles (user_id, lark_record) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET lark_record = EXCLUDED.lark_record WHERE nt_profiles.lark_record IS DISTINCT FROM EXCLUDED.lark_record`, uid, record)
	return err
}

func (r *ntRepository) LarkRecord(ctx context.Context, uid int64) (string, error) {
	var rec string
	err := r.db.QueryRow(ctx, `SELECT lark_record FROM nt_profiles WHERE user_id = $1`, uid).Scan(&rec)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return rec, err
}

func (r *ntRepository) SwapLock(ctx context.Context, uid int64, from, to int) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE users SET locked = $3, token_version = token_version + 1
		WHERE id = $1 AND role = 'nhathau' AND locked = $2`, uid, from, to)
	return err == nil && tag.RowsAffected() > 0, err
}

func (r *ntRepository) UserPhone(ctx context.Context, uid int64) (string, error) {
	var p string
	err := r.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1 AND role = 'nhathau'`, uid).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return p, err
}
