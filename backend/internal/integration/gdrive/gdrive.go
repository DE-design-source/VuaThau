// Package gdrive: lưu mỗi hồ sơ gửi duyệt thành 1 thư mục con trên Google Drive (ảnh gốc + thong-tin.txt).
// Gửi lại sau khi sửa: thư mục cũ của hồ sơ vào thùng rác. Dùng OAuth refresh token của 1 tài khoản Google.
package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	filesAPI  = "https://www.googleapis.com/drive/v3/files"
	uploadAPI = "https://www.googleapis.com/upload/drive/v3/files"
	allDrives = "supportsAllDrives=true"
)

type Client struct {
	clientID, clientSecret, refreshToken, folder string
	http                                         *http.Client
}

func New(clientID, clientSecret, refreshToken, folder string) *Client {
	return &Client{clientID: clientID, clientSecret: clientSecret, refreshToken: refreshToken, folder: folder,
		http: &http.Client{Timeout: 2 * time.Minute}}
}

// File: 1 file tải lên thư mục hồ sơ.
type File struct {
	Name string
	Mime string
	Data []byte
}

func (c *Client) call(ctx context.Context, method, u, token string, body []byte, ctype string, dst any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		if len(raw) > 300 {
			raw = raw[:300]
		}
		return nil, fmt.Errorf("%s %.90s: %d %s", method, u, res.StatusCode, raw)
	}
	if dst != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, dst); err != nil {
			return nil, err
		}
	}
	return res.Header, nil
}

func (c *Client) jsonCall(ctx context.Context, method, u, token string, body any, dst any) (http.Header, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.call(ctx, method, u, token, b, "application/json; charset=UTF-8", dst)
}

func (c *Client) token(ctx context.Context) (string, error) {
	form := url.Values{"client_id": {c.clientID}, "client_secret": {c.clientSecret},
		"refresh_token": {c.refreshToken}, "grant_type": {"refresh_token"}}
	var r struct {
		AccessToken string `json:"access_token"`
	}
	if _, err := c.call(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", "", []byte(form.Encode()),
		"application/x-www-form-urlencoded", &r); err != nil {
		return "", err
	}
	return r.AccessToken, nil
}

// PutHouse tạo thư mục cho hồ sơ houseID (tên folderName) và tải các file lên. Trả id thư mục.
func (c *Client) PutHouse(ctx context.Context, houseID int64, folderName string, files []File) (string, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return "", err
	}
	hs := strconv.FormatInt(houseID, 10)
	q := fmt.Sprintf("'%s' in parents and appProperties has { key='hs' and value='%s' } and trashed=false", c.folder, hs)
	var old struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if _, err := c.call(ctx, http.MethodGet, filesAPI+"?"+allDrives+"&includeItemsFromAllDrives=true&corpora=allDrives&fields=files(id)&"+
		url.Values{"q": {q}}.Encode(), tok, nil, "", &old); err != nil {
		return "", err
	}
	for _, f := range old.Files {
		if _, err := c.jsonCall(ctx, http.MethodPatch, filesAPI+"/"+f.ID+"?"+allDrives, tok, map[string]any{"trashed": true}, nil); err != nil {
			return "", err
		}
	}
	if r := []rune(folderName); len(r) > 200 {
		folderName = string(r[:200])
	}
	var folder struct {
		ID string `json:"id"`
	}
	if _, err := c.jsonCall(ctx, http.MethodPost, filesAPI+"?"+allDrives, tok, map[string]any{
		"name": folderName, "parents": []string{c.folder},
		"mimeType": "application/vnd.google-apps.folder", "appProperties": map[string]string{"hs": hs},
	}, &folder); err != nil {
		return "", err
	}
	for _, f := range files { // resumable 1 lần PUT: không vướng giới hạn 5MB của multipart
		h, err := c.jsonCall(ctx, http.MethodPost, uploadAPI+"?uploadType=resumable&"+allDrives, tok,
			map[string]any{"name": f.Name, "parents": []string{folder.ID}}, nil)
		if err != nil {
			return "", err
		}
		if _, err := c.call(ctx, http.MethodPut, h.Get("Location"), tok, f.Data, f.Mime, nil); err != nil {
			return "", err
		}
	}
	return folder.ID, nil
}
