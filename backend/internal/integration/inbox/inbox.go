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
	ConvID    string // hội thoại Messenger của hồ sơ
	CreatedAt time.Time
	UpdatedAt time.Time
	Fields    map[string]string // dia_chi, nha_thau, sdt_nha_thau, ngay_chup, toa_do, loai_cong_trinh, tien_do...
	Contact   Contact
	Images    []Image
	Videos    []string // khoá file video trong kho S3
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
	// Videos: khoá file video theo mã hồ sơ (project_logs), để kéo video cho hồ sơ đã nhập.
	Videos(ctx context.Context, logIDs []string) (map[string][]string, error)
	// Open: đọc dần 1 file (video), kèm kiểu file.
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	// Contacts: thông tin đầy đủ của người gửi bên Messenger (theo contacts.id), để đồng bộ hồ sơ CTV.
	Contacts(ctx context.Context, ids []string) (map[string]ContactInfo, error)
	Close()
}

// ContactInfo: toàn bộ thông tin 1 người gửi bên dezon-inbox (khung "Thông tin khách").
type ContactInfo struct {
	Name      string            `json:"name"`
	Phone     string            `json:"phone"`
	Email     string            `json:"email"`
	Avatar    string            `json:"avatar"`
	Tags      []string          `json:"tags"`
	Notes     string            `json:"notes"`
	Fields    map[string]string `json:"fields"` // khoá -> giá trị: need, khu_vuc, so_tai_khoan, loai_hinh_doi_tac...
	Labels    map[string]string `json:"labels"` // khoá -> nhãn hiển thị ("Nhu cầu", "Khu vực"...)
	UpdatedAt time.Time         `json:"updated_at"`
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
		SELECT pl.id::text, cv.id::text, pl.created_at, pl.updated_at, pl.fields, pl.message_ids::text[],
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
		if err := rows.Scan(&r.l.ID, &r.l.ConvID, &r.l.CreatedAt, &r.l.UpdatedAt, &fields, &r.msgs,
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
		// Chỉ lấy ảnh chatbot gắn vào hồ sơ. (19/11: từng thử gom thêm ảnh lẻ CTV gửi trước đó, bỏ vì hay gộp nhầm
		// ảnh của công trình khác; hồ sơ thiếu / gộp thì admin dùng nút Tách hồ sơ.)
		r.l.Images = imgs
		out = append(out, r.l)
	}
	if len(out) > 0 {
		ids := make([]string, len(out))
		for i, l := range out {
			ids[i] = l.ID
		}
		vs, err := c.Videos(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Videos = vs[out[i].ID]
		}
	}
	return out, nil
}

func (c *client) Videos(ctx context.Context, logIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(logIDs) == 0 {
		return out, nil
	}
	rows, err := c.db.Query(ctx, `
		SELECT pl.id::text, a->>'storage_key'
		FROM project_logs pl
		JOIN messages m ON m.id::text = ANY(pl.message_ids::text[])
		CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(m.attachments) = 'array' THEN m.attachments ELSE '[]' END) a
		WHERE pl.id::text = ANY($1) AND a->>'type' = 'video' AND COALESCE(a->>'storage_key', '') <> ''
		ORDER BY pl.id, COALESCE(m.sent_at, m.created_at), m.id`, logIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, err
		}
		out[id] = append(out[id], key)
	}
	return out, rows.Err()
}

func (c *client) Contacts(ctx context.Context, ids []string) (map[string]ContactInfo, error) {
	out := map[string]ContactInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := c.db.Query(ctx, `
		SELECT id::text, display_name, COALESCE(phone, ''), COALESCE(email, ''), COALESCE(avatar_url, ''), tags,
			COALESCE(notes, ''), COALESCE(profile_fields, '{}'::jsonb), updated_at
		FROM contacts WHERE id::text = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var ci ContactInfo
		var raw []byte
		if err := rows.Scan(&id, &ci.Name, &ci.Phone, &ci.Email, &ci.Avatar, &ci.Tags, &ci.Notes, &raw, &ci.UpdatedAt); err != nil {
			return nil, err
		}
		ci.Fields, ci.Labels = map[string]string{}, map[string]string{}
		var obj map[string]struct{ Label, Value string }
		if json.Unmarshal(raw, &obj) == nil {
			for k, v := range obj {
				if s := strings.TrimSpace(v.Value); s != "" {
					ci.Fields[k] = s
					ci.Labels[k] = strings.TrimSpace(v.Label)
				}
			}
		}
		if ci.Tags == nil {
			ci.Tags = []string{}
		}
		out[id] = ci
	}
	return out, rows.Err()
}

func (c *client) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	obj, err := c.s3.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	st, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, "", err
	}
	return obj, st.ContentType, nil
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
