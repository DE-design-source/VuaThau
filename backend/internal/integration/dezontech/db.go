package dezontech

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/province"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB: đọc khách hàng thẳng từ CSDL Dezon Pro (cùng máy chủ, mạng docker qspro_default) bằng tài khoản CHỈ ĐỌC
// vuathau_ro (chỉ có quyền SELECT bảng sales_leads; người dùng đồng ý 22/11/2026). DEZON_DB_URL.
type DB struct{ pool *pgxpool.Pool }

func NewDB(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close() { d.pool.Close() }

// Giai đoạn pipeline (mã bên Dezon Pro -> chữ hiện trên bản đồ)
var stageLabel = map[string]string{
	"tiep-can": "Tiếp cận", "giao-ns": "Giao nhân sự", "tu-van": "Tư vấn", "bao-gia": "Báo giá", "dam-phan": "Đàm phán",
	"chot-hd": "Chốt hợp đồng", "bao-gia-thi-cong": "Báo giá thi công", "thiet-ke": "Thiết kế", "thi-cong": "Thi công",
	"khong-tham-gia": "Không tham gia", "truot-thau": "Trượt thầu",
}

// leadExtra: ô Tỉnh thành (geo) và Trạng thái giao tiếp (rating) của khách: Dezon Pro lưu theo công ty trong
// company_settings (khoá kdOptions), đọc qua view vuathau.lead_extra (chỉ 2 phần này). Chưa có view: bỏ qua.
type leadExtra struct {
	geo    map[string]struct{ Kv, Tinh string }
	rating map[string]int
}

func (d *DB) extra(ctx context.Context) leadExtra {
	x := leadExtra{geo: map[string]struct{ Kv, Tinh string }{}, rating: map[string]int{}}
	rows, err := d.pool.Query(ctx, `SELECT COALESCE(geo, '{}'::jsonb)::text, COALESCE(rating, '{}'::jsonb)::text FROM vuathau.lead_extra`)
	if err != nil {
		return x
	}
	defer rows.Close()
	for rows.Next() {
		var g, r string
		if rows.Scan(&g, &r) != nil {
			continue
		}
		var gm map[string]struct {
			Kv   string `json:"kv"`
			Tinh string `json:"tinh"`
		}
		if json.Unmarshal([]byte(g), &gm) == nil {
			for k, v := range gm {
				x.geo[k] = struct{ Kv, Tinh string }{v.Kv, v.Tinh}
			}
		}
		var rm map[string]float64
		if json.Unmarshal([]byte(r), &rm) == nil {
			for k, v := range rm {
				if v >= 1 && v <= 5 {
					x.rating[k] = int(v)
				}
			}
		}
	}
	return x
}

// Leads: mọi khách trừ spam.
func (d *DB) Leads(ctx context.Context) ([]Lead, error) {
	extra := d.extra(ctx)
	rows, err := d.pool.Query(ctx, `SELECT id, customer_name, phone, email, address, lead_source, stage, project_type, scale_note,
			estimated_value_vnd::text, note, creator_name, manager_name, created_at
		FROM sales_leads WHERE stage <> 'spam' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		var (
			id                                                         int64
			name, phone, email, addr, src, stage, kind, scale, val, nt string
			creator, manager                                           string
			at                                                         time.Time
		)
		if err := rows.Scan(&id, &name, &phone, &email, &addr, &src, &stage, &kind, &scale, &val, &nt, &creator, &manager, &at); err != nil {
			return nil, err
		}
		if l, ok := stageLabel[stage]; ok {
			stage = l
		}
		l := Lead{ID: json.RawMessage(strconv.FormatInt(id, 10)), Name: name, Phone: phone, Email: email, Address: addr,
			Source: src, Stage: stage, ProjectType: kind, Scale: scale, ValueVnd: json.Number(val), Notes: nt,
			Creator: creator, Manager: manager, CreatedAt: at.Format(time.RFC3339Nano)}
		key := strconv.FormatInt(id, 10)
		if g, ok := extra.geo[key]; ok && (g.Kv != "" || g.Tinh != "") {
			l.Geo.Kv, l.Geo.Tinh = g.Kv, g.Tinh // ô Tỉnh thành bên Dezon Pro
		} else if strings.TrimSpace(addr) == "" {
			l.Geo.Kv, l.Geo.Tinh = areaFromNote(nt)
		}
		l.Rating = extra.rating[key]
		out = append(out, l)
	}
	return out, rows.Err()
}

// StaffNames: bảng khách đã có tên người phụ trách (manager_name), không cần đổi mã.
func (d *DB) StaffNames(context.Context) map[string]string { return map[string]string{} }

// Khách chưa ghi địa chỉ: ghi chú thường có dòng "Địa chỉ: Hà Nội", "ĐC: TP.HCM", "Khu vực: ..." hoặc nhắc tên tỉnh
// ("quán Cà phê Bắc Ninh"). Trả (khu vực theo dòng địa chỉ, tên tỉnh dò được) để đặt ghim gần đúng.
var noteAddr = regexp.MustCompile(`(?im)^\s*(?:địa chỉ|đ/c|đc|khu vực|vị trí|tỉnh|thành phố)\s*[:\-–]\s*(.+)$`)

func areaFromNote(note string) (kv, tinh string) {
	if m := noteAddr.FindStringSubmatch(note); m != nil {
		kv = strings.TrimSpace(strings.TrimRight(m[1], " .,;"))
		if len([]rune(kv)) > 120 {
			kv = string([]rune(kv)[:120])
		}
	}
	src := kv
	if src == "" {
		src = note
	}
	if p := province.FromAddress(strings.ReplaceAll(src, "\n", ", ")); p != nil {
		tinh = p.Name
	}
	if kv != "" && tinh != "" && strings.Contains(province.Key(kv), province.Key(strings.TrimPrefix(tinh, "TP. "))) {
		tinh = "" // dòng địa chỉ đã có tên tỉnh
	}
	return kv, tinh
}
