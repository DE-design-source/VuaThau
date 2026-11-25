package inbox

// Đọc hồ sơ Messenger qua API riêng của UniInbox (dezon-inbox) thay vì đọc thẳng DB / kho S3 của nó (góp ý
// 25/11/2026): GET /integrations/vuathau/{project-logs,contacts,media}, khoá INBOX_API_KEY, gọi qua mạng nội bộ
// docker (http://dezon-inbox-api:4000). UniInbox báo sang khi bot ghi hồ sơ mới (POST /api/v1/integrations/uniinbox/
// notify, handler.InboxNotify) ⇒ đồng bộ ngay; vòng INBOX_SYNC_EVERY chỉ còn để lấy bù.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type APIConfig struct {
	BaseURL string // http://dezon-inbox-api:4000
	Key     string // INBOX_API_KEY (= VUATHAU_API_KEY bên UniInbox)
}

type apiClient struct {
	base, key string
	http      *http.Client // gọi JSON (ngắn)
	files     *http.Client // tải ảnh / video (lâu hơn)
}

// NewAPI: nguồn hồ sơ qua API của UniInbox.
func NewAPI(c APIConfig) (Source, error) {
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("INBOX_API_URL không hợp lệ")
	}
	if len(c.Key) < 32 {
		return nil, fmt.Errorf("INBOX_API_KEY phải dài ít nhất 32 ký tự")
	}
	return &apiClient{base: u.String(), key: c.Key,
		http: &http.Client{Timeout: 30 * time.Second}, files: &http.Client{Timeout: 10 * time.Minute}}, nil
}

func (c *apiClient) get(ctx context.Context, cl *http.Client, path string, q url.Values) (*http.Response, error) {
	u := c.base + "/integrations/vuathau/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	res, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("UniInbox %s: %w", path, err)
	}
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		_ = res.Body.Close()
		return nil, fmt.Errorf("UniInbox %s: %d %s", path, res.StatusCode, strings.TrimSpace(string(b)))
	}
	return res, nil
}

func (c *apiClient) getJSON(ctx context.Context, path string, q url.Values, dst any) error {
	res, err := c.get(ctx, c.http, path, q)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(dst); err != nil {
		return fmt.Errorf("UniInbox %s: đọc JSON: %w", path, err)
	}
	return nil
}

type apiField struct{ Key, Label, Value string }

type apiProfile map[string]struct{ Label, Value string }

type apiMedia struct {
	Key    string    `json:"key"`
	Mime   string    `json:"mime"`
	SentAt time.Time `json:"sent_at"`
}

type apiLog struct {
	ID        string     `json:"id"`
	ConvID    string     `json:"conversation_id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Fields    []apiField `json:"fields"`
	Contact   struct {
		ID      string     `json:"id"`
		Name    string     `json:"name"`
		Phone   string     `json:"phone"`
		Profile apiProfile `json:"profile_fields"`
	} `json:"contact"`
	Images []apiMedia `json:"images"`
	Videos []apiMedia `json:"videos"`
}

func (p apiProfile) values() map[string]string {
	out := map[string]string{}
	for k, v := range p {
		if s := strings.TrimSpace(v.Value); s != "" {
			out[k] = s
		}
	}
	return out
}

func (c *apiClient) Logs(ctx context.Context, afterAt time.Time, afterID string, limit int) ([]Log, error) {
	q := url.Values{"limit": {fmt.Sprint(min(max(limit, 1), 200))}, "after_id": {afterID}}
	if !afterAt.IsZero() {
		q.Set("after_at", afterAt.UTC().Format(time.RFC3339Nano))
	}
	var res struct{ Items []apiLog }
	if err := c.getJSON(ctx, "project-logs", q, &res); err != nil {
		return nil, err
	}
	out := make([]Log, 0, len(res.Items))
	for _, x := range res.Items {
		l := Log{ID: x.ID, ConvID: x.ConvID, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt, Fields: map[string]string{},
			Contact: Contact{ID: x.Contact.ID, Name: x.Contact.Name, Phone: x.Contact.Phone, Profile: x.Contact.Profile.values()}}
		for _, f := range x.Fields {
			if v := strings.TrimSpace(f.Value); v != "" {
				l.Fields[f.Key] = v
			}
		}
		for _, m := range x.Images {
			l.Images = append(l.Images, Image{Key: m.Key, Mime: m.Mime, SentAt: m.SentAt})
		}
		for _, m := range x.Videos {
			l.Videos = append(l.Videos, m.Key)
		}
		out = append(out, l)
	}
	return out, nil
}

type apiPhotoMsg struct {
	ID           string    `json:"id"`
	ConvID       string    `json:"conversation_id"`
	SentAt       time.Time `json:"sent_at"`
	InProjectLog bool      `json:"in_project_log"`
	Contact      struct {
		ID      string     `json:"id"`
		Name    string     `json:"name"`
		Phone   string     `json:"phone"`
		Profile apiProfile `json:"profile_fields"`
	} `json:"contact"`
	Images []struct {
		Key  string `json:"key"`
		Mime string `json:"mime"`
	} `json:"images"`
}

func (c *apiClient) PhotoMessages(ctx context.Context, since, afterAt time.Time, afterID string, limit int) ([]PhotoMessage, error) {
	q := url.Values{"since": {since.UTC().Format(time.RFC3339Nano)}, "limit": {fmt.Sprint(min(max(limit, 1), 500))},
		"after_id": {afterID}}
	if !afterAt.IsZero() {
		q.Set("after_at", afterAt.UTC().Format(time.RFC3339Nano))
	}
	var res struct{ Items []apiPhotoMsg }
	if err := c.getJSON(ctx, "photo-messages", q, &res); err != nil {
		return nil, err
	}
	out := make([]PhotoMessage, 0, len(res.Items))
	for _, x := range res.Items {
		m := PhotoMessage{ID: x.ID, ConvID: x.ConvID, SentAt: x.SentAt, InLog: x.InProjectLog,
			Contact: Contact{ID: x.Contact.ID, Name: x.Contact.Name, Phone: x.Contact.Phone, Profile: x.Contact.Profile.values()}}
		for _, im := range x.Images {
			m.Images = append(m.Images, Image{Key: im.Key, Mime: im.Mime, SentAt: x.SentAt})
		}
		out = append(out, m)
	}
	return out, nil
}

func (c *apiClient) Image(ctx context.Context, key string) ([]byte, error) {
	res, err := c.get(ctx, c.files, "media", url.Values{"key": {key}})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(io.LimitReader(res.Body, 25<<20))
}

// Videos: API trả video kèm từng hồ sơ (Logs). Chỉ lượt kéo bù video cũ (inbox_videos_v1, đã chạy xong 18/11/2026)
// gọi hàm này: không còn gì để kéo.
func (c *apiClient) Videos(context.Context, []string) (map[string][]string, error) {
	return map[string][]string{}, nil
}

func (c *apiClient) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	res, err := c.get(ctx, c.files, "media", url.Values{"key": {key}})
	if err != nil {
		return nil, "", err
	}
	return res.Body, res.Header.Get("Content-Type"), nil
}

func (c *apiClient) Contacts(ctx context.Context, ids []string) (map[string]ContactInfo, error) {
	out := map[string]ContactInfo{}
	for len(ids) > 0 {
		chunk := ids[:min(200, len(ids))]
		ids = ids[len(chunk):]
		var res struct {
			Items map[string]struct {
				Name      string     `json:"name"`
				Phone     string     `json:"phone"`
				Email     string     `json:"email"`
				Avatar    string     `json:"avatar"`
				Tags      []string   `json:"tags"`
				Notes     string     `json:"notes"`
				Profile   apiProfile `json:"profile_fields"`
				UpdatedAt time.Time  `json:"updated_at"`
			}
		}
		if err := c.getJSON(ctx, "contacts", url.Values{"ids": {strings.Join(chunk, ",")}}, &res); err != nil {
			return nil, err
		}
		for id, x := range res.Items {
			ci := ContactInfo{Name: x.Name, Phone: x.Phone, Email: x.Email, Avatar: x.Avatar, Tags: x.Tags, Notes: x.Notes,
				Fields: map[string]string{}, Labels: map[string]string{}, UpdatedAt: x.UpdatedAt}
			for k, v := range x.Profile {
				if s := strings.TrimSpace(v.Value); s != "" {
					ci.Fields[k] = s
					ci.Labels[k] = strings.TrimSpace(v.Label)
				}
			}
			if ci.Tags == nil {
				ci.Tags = []string{}
			}
			out[id] = ci
		}
	}
	return out, nil
}

func (c *apiClient) Close() {}
