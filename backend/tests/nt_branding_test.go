package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
)

// Logo trên ảnh (thiết kế 24/11/2026): tên thương hiệu tối đa 20 ký tự, cỡ, nhãn Vua Thầu xác minh; tải logo PNG
// (giữ nguyên file, nền trong), app tải logo về in lên ảnh; bỏ logo.
func TestNhaThauBranding(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	phone := e.newPhone()
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken, map[string]string{"phone": phone, "name": "Smartcons"}), http.StatusOK)
	var lg struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone, "as": "nhathau"}), &lg)
	tok := lg.AccessToken

	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/nt/branding", tok, map[string]any{"brand": "Tên thương hiệu quá dài hơn hai mươi", "size": 2, "badge": true}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/nt/branding", tok, map[string]any{"brand": "Smartcons", "size": 3, "badge": false}), http.StatusOK)

	// logo PNG 320x320 nền trong
	img := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	img.Set(10, 10, color.NRGBA{R: 255, A: 255})
	var pb bytes.Buffer
	_ = png.Encode(&pb, img)
	logo := pb.Bytes()
	var mb bytes.Buffer
	mw := multipart.NewWriter(&mb)
	fw, _ := mw.CreateFormFile("file", "logo.png")
	_, _ = fw.Write(logo)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1/nt/logo", &mb)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	expectStatus(t, e.send(t, req), http.StatusOK)

	var home struct {
		Profile struct {
			Brand     string `json:"brand"`
			HasLogo   bool   `json:"has_logo"`
			LogoSize  int    `json:"logo_size"`
			ShowBadge bool   `json:"show_badge"`
		} `json:"profile"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", tok, nil), &home)
	if home.Profile.Brand != "Smartcons" || !home.Profile.HasLogo || home.Profile.LogoSize != 3 || home.Profile.ShowBadge {
		t.Fatalf("thương hiệu: %+v", home.Profile)
	}
	// app tải logo về: đúng file PNG đã gửi (giữ nền trong)
	gr, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/nt/logo", nil)
	gr.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(gr)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("tải logo: %v %+v", err, res)
	}
	got, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !bytes.Equal(got, logo) {
		t.Fatalf("logo khác file đã gửi (%d / %d byte)", len(got), len(logo))
	}
	// bỏ logo
	expectStatus(t, e.do(t, http.MethodDelete, "/api/v1/nt/logo", tok, nil), http.StatusOK)
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", tok, nil), &home)
	if home.Profile.HasLogo || home.Profile.Brand != "Smartcons" {
		t.Fatalf("bỏ logo: %+v", home.Profile)
	}
}
