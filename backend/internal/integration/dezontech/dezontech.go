// Package dezontech đọc khách hàng (pipeline Kinh doanh) của Dezon Pro: GET {URL}/api/v1/sales/leads
// với khoá chỉ đọc (Authorization: Bearer <DEZON_TECH_KEY>), cùng dạng dữ liệu web Dezon Pro dùng ({ leads: [...] }).
// Người phụ trách: GET /api/v1/sales/staff ({ staff: [...] }) để đổi mã nhân viên ra tên (lỗi: giữ nguyên mã).
package dezontech

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	url, key string
	http     *http.Client
}

// New: key trống thì trả nil (tắt lớp khách hàng).
func New(url, key string) *Client {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	return &Client{url: strings.TrimRight(url, "/"), key: strings.TrimSpace(key), http: &http.Client{Timeout: 30 * time.Second}}
}

// Lead: 1 khách hàng bên Dezon Pro (chỉ các ô Vua Thầu dùng; ô lạ bỏ qua).
type Lead struct {
	ID          json.RawMessage           `json:"id"`
	Name        string                    `json:"name"`
	Phone       string                    `json:"phone"`
	Email       string                    `json:"email"`
	Address     string                    `json:"address"`
	ProjName    string                    `json:"projName"`
	Geo         struct{ Kv, Tinh string } `json:"geo"`
	ProjectType string                    `json:"projectType"`
	Scale       string                    `json:"scale"`
	ValueVnd    json.Number               `json:"valueVnd"`
	Stage       string                    `json:"stage"`
	Source      string                    `json:"source"`
	Manager     any                       `json:"manager"`
	Creator     any                       `json:"creator"`
	Notes       string                    `json:"notes"`
	CreatedAt   string                    `json:"createdAt"`
}

// Key: mã khách dạng chữ (id bên kia có thể là số hoặc chữ).
func (l Lead) Key() string {
	s := strings.Trim(string(l.ID), `"`)
	return strings.TrimSpace(s)
}

// Who: mã / tên người phụ trách (manager, không có thì người tạo).
func (l Lead) Who() string {
	for _, v := range []any{l.Manager, l.Creator} {
		switch x := v.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				return strings.TrimSpace(x)
			}
		case float64:
			return fmt.Sprintf("%.0f", x)
		case map[string]any:
			for _, k := range []string{"name", "fullName", "username", "id"} {
				if s, ok := x[k].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

var ErrAuth = errors.New("dezon.tech từ chối khoá (DEZON_TECH_KEY sai hoặc hết hạn)")

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/api/v1"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	raw := json.NewDecoder(res.Body)
	var all json.RawMessage
	if err := raw.Decode(&all); err != nil {
		return fmt.Errorf("dezon.tech %s: HTTP %d, không đọc được: %w", path, res.StatusCode, err)
	}
	_ = json.Unmarshal(all, &body)
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden || body.Code == "NOAUTH" {
		return ErrAuth
	}
	if res.StatusCode != http.StatusOK || body.Error != "" {
		return fmt.Errorf("dezon.tech %s: HTTP %d %s", path, res.StatusCode, body.Error)
	}
	return json.Unmarshal(all, out)
}

// Leads: toàn bộ khách hàng.
func (c *Client) Leads(ctx context.Context) ([]Lead, error) {
	var r struct {
		Leads []Lead `json:"leads"`
	}
	if err := c.get(ctx, "/sales/leads", &r); err != nil {
		return nil, err
	}
	return r.Leads, nil
}

// StaffNames: mã nhân viên -> tên (để hiện người phụ trách). Lỗi: map rỗng.
func (c *Client) StaffNames(ctx context.Context) map[string]string {
	var r struct {
		Staff []map[string]any `json:"staff"`
	}
	out := map[string]string{}
	if err := c.get(ctx, "/sales/staff", &r); err != nil {
		return out
	}
	str := func(m map[string]any, ks ...string) string {
		for _, k := range ks {
			switch v := m[k].(type) {
			case string:
				if v != "" {
					return v
				}
			case float64:
				return fmt.Sprintf("%.0f", v)
			}
		}
		return ""
	}
	for _, s := range r.Staff {
		name := str(s, "name", "fullName", "displayName")
		for _, k := range []string{"id", "username", "email", "code"} {
			if v := str(s, k); v != "" && name != "" {
				out[v] = name
			}
		}
	}
	return out
}
