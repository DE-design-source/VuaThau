package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Trang sale: chỉ công trình thật (đã gửi duyệt, không bị từ chối), chỉ thông tin công trình + nhà thầu.
// KHÔNG join bảng users, KHÔNG lấy tiền: sale không được thấy CTV và tiền.
const saleVisible = `h.status IN ('cho_duyet', 'tra_lai', 'da_duyet', 'da_chi')`

// SalePoint: 1 công trình trên bản đồ sale.
type SalePoint struct {
	ID         int64     `json:"id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Address    string    `json:"address"`
	Contractor string    `json:"contractor"`
	Stage      string    `json:"stage"`
	Province   string    `json:"province"`
	CTVName    string    `json:"ctv_name"` // chỉ tên CTV (sale lọc theo CTV); không SĐT, tiền, ngân hàng
	At         time.Time `json:"at"`
	LastAt     time.Time `json:"last_at"` // lần chụp gần nhất ở công trình này (service điền)
}

// SaleCTV: 1 CTV trong ô lọc của sale: chỉ tên và số công trình.
type SaleCTV struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Total int    `json:"total"`
}

// SaleProject: 1 dự án (công trình) trong danh sách / chi tiết của sale.
type SaleProject struct {
	ID              int64     `json:"id"`
	Address         string    `json:"address"`
	Lat             *float64  `json:"lat"`
	Lng             *float64  `json:"lng"`
	Province        string    `json:"province"`
	ProvinceName    string    `json:"province_name"`
	Floors          *int      `json:"floors"`
	Stage           string    `json:"stage"`
	ContractorName  string    `json:"contractor_name"`
	ContractorPhone string    `json:"contractor_phone"`
	At              time.Time `json:"at"`
	CoverID         *int64    `json:"cover_id"`
	Photos          int       `json:"photos"`
}

// SaleFilter: lọc danh sách dự án của sale.
type SaleFilter struct {
	Q          string // địa chỉ, nhà thầu, SĐT nhà thầu, mã
	Province   string
	Contractor string // khoá nhà thầu (Contractor.Key)
	CTVID      int64  // chỉ công trình của 1 CTV (bản đồ sale)
}

// Contractor: 1 nhà thầu gộp từ các công trình.
type Contractor struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Total     int       `json:"total"`
	Provinces []string  `json:"provinces"`
	Last      time.Time `json:"last"`
}

func (f SaleFilter) where() (string, []any) {
	w, args := []string{saleVisible}, []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		w = append(w, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Province != "" {
		add("h.province = ?", f.Province)
	}
	if f.CTVID > 0 {
		add("h.ctv_id = ?", f.CTVID)
	}
	if f.Contractor != "" {
		add(saleContractorKey+" = ?", f.Contractor)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		cond := fmt.Sprintf("(h.address ILIKE $%d OR h.contractor_name ILIKE $%d OR h.contractor_phone ILIKE $%d", n, n, n)
		// mã dự án: 12, #12, HS-00012
		if id, err := strconv.ParseInt(strings.TrimLeft(strings.TrimPrefix(strings.ToUpper(q), "HS-"), "#0"), 10, 64); err == nil {
			args = append(args, id)
			cond += fmt.Sprintf(" OR h.id = $%d", len(args))
		}
		w = append(w, cond+")")
	}
	return strings.Join(w, " AND "), args
}

// saleContractorKey: khoá gộp nhà thầu (SĐT chỉ chữ số, không có thì "ten:" + tên viết thường).
const saleContractorKey = `COALESCE(NULLIF(regexp_replace(h.contractor_phone, '\D', '', 'g'), ''), 'ten:' || lower(btrim(h.contractor_name)))`

func (r *houseRepository) SalePoints(ctx context.Context, f SaleFilter) ([]SalePoint, error) {
	where, args := f.where()
	rows, err := r.db.Query(ctx, `SELECT h.id, h.lat, h.lng, h.address, h.contractor_name, h.stage, COALESCE(h.province, ''),
		COALESCE(u.name, ''), COALESCE(h.submitted_at, h.created_at) FROM houses h LEFT JOIN users u ON u.id = h.ctv_id
		WHERE `+where+` AND h.lat IS NOT NULL AND h.lng IS NOT NULL
		ORDER BY h.id DESC LIMIT 20000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SalePoint{}
	for rows.Next() {
		var p SalePoint
		if err := rows.Scan(&p.ID, &p.Lat, &p.Lng, &p.Address, &p.Contractor, &p.Stage, &p.Province, &p.CTVName, &p.At); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SaleProvinceCounts: số công trình mỗi tỉnh (mã tỉnh → số).
func (r *houseRepository) SaleProvinceCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT h.province, COUNT(*) FROM houses h WHERE `+saleVisible+` AND h.province IS NOT NULL
		AND h.province <> '' GROUP BY h.province`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		out[code] = n
	}
	return out, rows.Err()
}

const saleProjectCols = `h.id, h.address, h.lat, h.lng, COALESCE(h.province, ''), h.floors, h.stage, h.contractor_name, h.contractor_phone,
	COALESCE(h.submitted_at, h.created_at), (SELECT MIN(p.id) FROM photos p WHERE p.house_id = h.id),
	(SELECT COUNT(*) FROM photos p WHERE p.house_id = h.id)`

func scanSaleProject(row interface{ Scan(...any) error }) (*SaleProject, error) {
	var p SaleProject
	if err := row.Scan(&p.ID, &p.Address, &p.Lat, &p.Lng, &p.Province, &p.Floors, &p.Stage, &p.ContractorName, &p.ContractorPhone,
		&p.At, &p.CoverID, &p.Photos); err != nil {
		return nil, err
	}
	return &p, nil
}

// SaleProjects: 1 trang dự án mới nhất trước + tổng số khớp bộ lọc.
func (r *houseRepository) SaleProjects(ctx context.Context, f SaleFilter, limit, offset int) ([]SaleProject, int, error) {
	where, args := f.where()
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM houses h WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `SELECT `+saleProjectCols+` FROM houses h WHERE `+where+
		fmt.Sprintf(` ORDER BY COALESCE(h.submitted_at, h.created_at) DESC, h.id DESC LIMIT %d OFFSET %d`, limit, offset), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []SaleProject{}
	for rows.Next() {
		p, err := scanSaleProject(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	return out, total, rows.Err()
}

// SaleProject: 1 dự án sale được xem; ErrNotFound nếu không có hoặc không được xem (nháp, bị từ chối).
func (r *houseRepository) SaleProject(ctx context.Context, id int64) (*SaleProject, error) {
	p, err := scanSaleProject(r.db.QueryRow(ctx, `SELECT `+saleProjectCols+` FROM houses h WHERE h.id = $1 AND `+saleVisible, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// SaleCanSeeHouse: sale được xem ảnh của hồ sơ này không.
func (r *houseRepository) SaleCanSeeHouse(ctx context.Context, id int64) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM houses h WHERE h.id = $1 AND `+saleVisible+`)`, id).Scan(&ok)
	return ok, err
}

// Contractors: nhà thầu gộp theo SĐT (hoặc tên khi không có SĐT), nhiều công trình nhất trước.
func (r *houseRepository) Contractors(ctx context.Context, q string) ([]Contractor, error) {
	where, args := SaleFilter{Q: q}.where()
	rows, err := r.db.Query(ctx, `SELECT `+saleContractorKey+` AS k,
			(array_agg(h.contractor_name ORDER BY (h.contractor_name <> '') DESC, h.id DESC))[1],
			(array_agg(h.contractor_phone ORDER BY (h.contractor_phone <> '') DESC, h.id DESC))[1],
			COUNT(*), array_remove(array_agg(DISTINCT h.province), NULL), MAX(COALESCE(h.submitted_at, h.created_at))
		FROM houses h WHERE `+where+` AND (btrim(h.contractor_name) <> '' OR btrim(h.contractor_phone) <> '')
		GROUP BY k ORDER BY 4 DESC, 6 DESC LIMIT 2000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contractor{}
	for rows.Next() {
		var c Contractor
		if err := rows.Scan(&c.Key, &c.Name, &c.Phone, &c.Total, &c.Provinces, &c.Last); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaleCTVs: CTV có công trình sale được xem (ô lọc CTV): chỉ id, tên, số công trình.
func (r *houseRepository) SaleCTVs(ctx context.Context) ([]SaleCTV, error) {
	rows, err := r.db.Query(ctx, `SELECT u.id, u.name, COUNT(*) FROM houses h JOIN users u ON u.id = h.ctv_id
		WHERE `+saleVisible+` GROUP BY u.id, u.name ORDER BY 3 DESC, u.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SaleCTV{}
	for rows.Next() {
		var c SaleCTV
		if err := rows.Scan(&c.ID, &c.Name, &c.Total); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
