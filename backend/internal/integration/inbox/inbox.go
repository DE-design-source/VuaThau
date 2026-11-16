// Package inbox đọc hồ sơ công trình CTV gửi qua Messenger từ hệ thống dezon-inbox:
// chatbot "Cập nhật công trình" ghi mỗi lượt CTV gửi ảnh thành 1 dòng project_logs (địa chỉ, nhà thầu, toạ độ...),
// ảnh nằm trong messages.attachments, file ảnh ở kho S3 (SeaweedFS) của dezon-inbox.
// Chỉ đọc: mọi giao dịch trên DB dezon-inbox đặt default_transaction_read_only.
package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Log: 1 hồ sơ công trình chatbot đã ghi.
type Log struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Fields    map[string]string // dia_chi, nha_thau, sdt_nha_thau, ngay_chup, toa_do, loai_cong_trinh, tien_do...
	Contact   Contact
	Images    []Image
}

// Contact: người gửi (CTV) bên dezon-inbox.
type Contact struct {
	ID      string
	Name    string
	Phone   string
	Profile map[string]string // ô bot ghi nhận: need, khu_vuc, so_tai_khoan...
}

type Image struct {
	Key    string // khoá file trong kho S3
	Mime   string
	SentAt time.Time
}

// Source: nơi lấy hồ sơ Messenger (test thay bằng bản giả).
type Source interface {
	// Logs: các hồ sơ cập nhật sau mốc (updated_at, id), tăng dần, tối đa limit.
	Logs(ctx context.Context, afterAt time.Time, afterID string, limit int) ([]Log, error)
	Image(ctx context.Context, key string) ([]byte, error)
	Close()
}

type Config struct {
	DatabaseURL string
	S3Endpoint  string // http://dezon-inbox-s3:8333
	S3Bucket    string
	S3Access    string
	S3Secret    string
	S3Region    string
}

type client struct {
	db     *pgxpool.Pool
	s3     *minio.Client
	bucket string
}

func New(ctx context.Context, c Config) (Source, error) {
	pc, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("INBOX_DATABASE_URL: %w", err)
	}
	pc.MaxConns = 2
	pc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pc.ConnConfig.RuntimeParams["application_name"] = "vuathau-nghiemthu"
	db, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(c.S3Endpoint)
	if err != nil || u.Host == "" {
		db.Close()
		return nil, fmt.Errorf("INBOX_S3_ENDPOINT không hợp lệ")
	}
	s3, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(c.S3Access, c.S3Secret, ""),
		Secure: u.Scheme == "https",
		Region: c.S3Region,
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &client{db: db, s3: s3, bucket: c.S3Bucket}, nil
}

func (c *client) Close() { c.db.Close() }

func (c *client) Logs(ctx context.Context, afterAt time.Time, afterID string, limit int) ([]Log, error) {
	rows, err := c.db.Query(ctx, `
		SELECT pl.id::text, pl.created_at, pl.updated_at, pl.fields, pl.message_ids::text[],
			ct.id::text, COALESCE(ct.display_name, ''), COALESCE(ct.phone, ''), COALESCE(ct.profile_fields, '{}'::jsonb)
		FROM project_logs pl
		JOIN conversations cv ON cv.id = pl.conversation_id
		JOIN contacts ct ON ct.id = cv.contact_id
		WHERE (pl.updated_at, pl.id::text) > ($1, $2)
		ORDER BY pl.updated_at, pl.id::text LIMIT $3`, afterAt, afterID, limit)
	if err != nil {
		return nil, err
	}
	type row struct {
		l    Log
		msgs []string
	}
	all := []row{}
	for rows.Next() {
		var r row
		var fields, profile []byte
		if err := rows.Scan(&r.l.ID, &r.l.CreatedAt, &r.l.UpdatedAt, &fields, &r.msgs,
			&r.l.Contact.ID, &r.l.Contact.Name, &r.l.Contact.Phone, &profile); err != nil {
			rows.Close()
			return nil, err
		}
		r.l.Fields = parseFields(fields)
		r.l.Contact.Profile = parseProfile(profile)
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Log, 0, len(all))
	for _, r := range all {
		imgs, err := c.images(ctx, r.msgs)
		if err != nil {
			return nil, err
		}
		r.l.Images = imgs
		out = append(out, r.l)
	}
	return out, nil
}

func (c *client) images(ctx context.Context, msgIDs []string) ([]Image, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	rows, err := c.db.Query(ctx, `
		SELECT a->>'storage_key', COALESCE(a->>'mime', 'image/jpeg'), COALESCE(m.sent_at, m.created_at)
		FROM messages m, jsonb_array_elements(CASE WHEN jsonb_typeof(m.attachments) = 'array' THEN m.attachments ELSE '[]' END) a
		WHERE m.id::text = ANY($1) AND a->>'type' = 'image' AND COALESCE(a->>'storage_key', '') <> ''
		ORDER BY COALESCE(m.sent_at, m.created_at), m.id`, msgIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Image, error) {
		var im Image
		err := r.Scan(&im.Key, &im.Mime, &im.SentAt)
		return im, err
	})
}

func (c *client) Image(ctx context.Context, key string) ([]byte, error) {
	obj, err := c.s3.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(io.LimitReader(obj, 25<<20))
}

// fields: [{"key": "dia_chi", "label": "...", "value": "..."}, ...]
func parseFields(raw []byte) map[string]string {
	out := map[string]string{}
	var list []struct{ Key, Value string }
	if json.Unmarshal(raw, &list) == nil {
		for _, f := range list {
			if v := strings.TrimSpace(f.Value); v != "" {
				out[f.Key] = v
			}
		}
		return out
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for k, v := range obj {
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return out
}

// profile_fields: {"need": {"label": "...", "value": "..."}, ...}
func parseProfile(raw []byte) map[string]string {
	out := map[string]string{}
	var obj map[string]struct{ Value string }
	if json.Unmarshal(raw, &obj) == nil {
		for k, v := range obj {
			if s := strings.TrimSpace(v.Value); s != "" {
				out[k] = s
			}
		}
	}
	return out
}
