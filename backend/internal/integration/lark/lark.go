// Package lark đọc bảng Lark Base "Form Tuyển CTV" (Open API của Lark, bản quốc tế open.larksuite.com):
// lấy tenant token bằng app id / secret, đổi trang wiki ra mã Base, rồi đọc toàn bộ bản ghi của 1 bảng.
// App Lark cần quyền đọc Base (bitable:app:readonly hoặc base:record:retrieve) và được thêm vào Base đó.
package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	api, appID, secret string
	wiki, table        string
	http               *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
	appToken string // mã Base (đổi từ trang wiki 1 lần)
}

// New: thiếu app id / secret thì trả nil (tắt).
func New(api, appID, secret, wiki, table string) *Client {
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(secret) == "" || wiki == "" || table == "" {
		return nil
	}
	return &Client{api: strings.TrimRight(api, "/"), appID: strings.TrimSpace(appID), secret: strings.TrimSpace(secret),
		wiki: wiki, table: table, http: &http.Client{Timeout: 30 * time.Second}}
}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, auth bool, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api+"/open-apis"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if auth {
		tok, err := c.tenantToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return fmt.Errorf("lark %s: HTTP %d", path, res.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	var e envelope
	_ = json.Unmarshal(raw, &e)
	if e.Code != 0 {
		return fmt.Errorf("lark %s: mã %d %s", path, e.Code, e.Msg)
	}
	return nil
}

func (c *Client) tenantToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		t := c.token
		c.mu.Unlock()
		return t, nil
	}
	c.mu.Unlock()
	var r struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Token  string `json:"tenant_access_token"`
		Expire int    `json:"expire"`
	}
	if err := c.do(ctx, http.MethodPost, "/auth/v3/tenant_access_token/internal", map[string]string{"app_id": c.appID, "app_secret": c.secret}, false, &r); err != nil {
		return "", err
	}
	if r.Token == "" {
		return "", errors.New("lark: không lấy được tenant token (kiểm tra LARK_APP_ID / LARK_APP_SECRET)")
	}
	c.mu.Lock()
	c.token, c.tokenExp = r.Token, time.Now().Add(time.Duration(max(r.Expire-300, 60))*time.Second)
	c.mu.Unlock()
	return r.Token, nil
}

// base: mã Base của trang wiki.
func (c *Client) base(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.appToken != "" {
		t := c.appToken
		c.mu.Unlock()
		return t, nil
	}
	c.mu.Unlock()
	var r struct {
		Data struct {
			Node struct {
				ObjToken string `json:"obj_token"`
				ObjType  string `json:"obj_type"`
			} `json:"node"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/wiki/v2/spaces/get_node?token="+url.QueryEscape(c.wiki), nil, true, &r); err != nil {
		return "", err
	}
	if r.Data.Node.ObjToken == "" {
		return "", errors.New("lark: trang wiki không phải Base, hoặc app chưa được thêm vào Base")
	}
	c.mu.Lock()
	c.appToken = r.Data.Node.ObjToken
	c.mu.Unlock()
	return r.Data.Node.ObjToken, nil
}

// Record: 1 dòng của bảng: tên cột -> giá trị (Lark trả nhiều dạng: chữ, mảng {text}, số, đúng / sai).
type Record struct {
	ID     string         `json:"record_id"`
	Fields map[string]any `json:"fields"`
}

// Records: toàn bộ dòng của bảng.
func (c *Client) Records(ctx context.Context) ([]Record, error) {
	app, err := c.base(ctx)
	if err != nil {
		return nil, err
	}
	out := []Record{}
	page := ""
	for i := 0; i < 50; i++ { // tối đa 25.000 dòng
		q := url.Values{"page_size": {"500"}}
		if page != "" {
			q.Set("page_token", page)
		}
		var r struct {
			Data struct {
				Items     []Record `json:"items"`
				HasMore   bool     `json:"has_more"`
				PageToken string   `json:"page_token"`
			} `json:"data"`
		}
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records?%s", app, c.table, q.Encode()), nil, true, &r); err != nil {
			return nil, err
		}
		out = append(out, r.Data.Items...)
		if !r.Data.HasMore || r.Data.PageToken == "" {
			break
		}
		page = r.Data.PageToken
	}
	return out, nil
}

// Text: giá trị 1 ô thành chữ (ô chữ, SĐT, chọn 1, mảng đoạn chữ, số).
func Text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", x), "0"), ".")
	case bool:
		if x {
			return "có"
		}
		return ""
	case []any:
		parts := []string{}
		for _, it := range x {
			switch y := it.(type) {
			case map[string]any:
				for _, k := range []string{"text", "name", "full_address", "value"} {
					if s, ok := y[k].(string); ok && s != "" {
						parts = append(parts, s)
						break
					}
				}
			default:
				if s := Text(y); s != "" {
					parts = append(parts, s)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case map[string]any:
		for _, k := range []string{"text", "full_address", "name", "value"} {
			if s, ok := x[k].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// Checked: ô tích (checkbox) đang được tích.
func Checked(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1"
	}
	return false
}
