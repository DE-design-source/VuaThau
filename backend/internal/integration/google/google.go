// Package google: xác minh id_token của Google Sign-In (app Android) qua tokeninfo của Google,
// nên máy chủ không cần thư viện mã hoá. Chỉ nhận token cấp cho đúng client id của mình.
package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// Identity: tài khoản Google đã xác minh.
type Identity struct {
	Sub   string `json:"-"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Verifier struct {
	clientIDs []string
	http      *http.Client
}

func NewVerifier(clientIDs []string) *Verifier {
	return &Verifier{clientIDs: clientIDs, http: &http.Client{Timeout: 10 * time.Second}}
}

// Enabled: đã cấu hình client id (GOOGLE_CLIENT_IDS).
func (v *Verifier) Enabled() bool { return len(v.clientIDs) > 0 }

// ClientID: web client id gửi cho app.
func (v *Verifier) ClientID() string {
	if len(v.clientIDs) == 0 {
		return ""
	}
	return v.clientIDs[0]
}

// Verify: id_token sai, hết hạn, của app khác, email chưa xác minh -> nil.
func (v *Verifier) Verify(ctx context.Context, idToken string) *Identity {
	if !v.Enabled() || idToken == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?"+url.Values{"id_token": {idToken}}.Encode(), nil)
	if err != nil {
		return nil
	}
	res, err := v.http.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var r struct {
		Aud           string `json:"aud"`
		Iss           string `json:"iss"`
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
	}
	if json.NewDecoder(res.Body).Decode(&r) != nil {
		return nil
	}
	verified := r.EmailVerified == true || r.EmailVerified == "true"
	if !slices.Contains(v.clientIDs, r.Aud) || (r.Iss != "accounts.google.com" && r.Iss != "https://accounts.google.com") ||
		!verified || r.Sub == "" {
		return nil
	}
	return &Identity{Sub: r.Sub, Email: r.Email, Name: r.Name}
}
