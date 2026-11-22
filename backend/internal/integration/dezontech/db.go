package dezontech

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

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

// Leads: mọi khách trừ spam.
func (d *DB) Leads(ctx context.Context) ([]Lead, error) {
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
		out = append(out, Lead{ID: json.RawMessage(strconv.FormatInt(id, 10)), Name: name, Phone: phone, Email: email, Address: addr,
			Source: src, Stage: stage, ProjectType: kind, Scale: scale, ValueVnd: json.Number(val), Notes: nt,
			Creator: creator, Manager: manager, CreatedAt: at.Format(time.RFC3339Nano)})
	}
	return out, rows.Err()
}

// StaffNames: bảng khách đã có tên người phụ trách (manager_name), không cần đổi mã.
func (d *DB) StaffNames(context.Context) map[string]string { return map[string]string{} }
