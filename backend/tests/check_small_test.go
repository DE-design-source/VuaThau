package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"net/http"
	"testing"

	"golang.org/x/image/draw"
)

// shrink: bản thu nhỏ app gửi đi kiểm khi mạng yếu (cạnh dài 1280, JPEG 75).
func shrink(t *testing.T, full []byte) []byte {
	t.Helper()
	src, err := jpeg.Decode(bytes.NewReader(full))
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	w, h := 1280, 1280*b.Dy()/b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}

// Mạng yếu (góp ý CTV 25/11/2026): app gửi bản thu nhỏ đi kiểm kèm sha256 ảnh gốc; nộp ảnh gốc thì máy chủ dùng
// lại kết quả AI đã đọc (không gọi AI lần 2), ảnh đã nộp mà kiểm lại (bản nhỏ + sha gốc) thì báo trùng.
func TestCheckSmallCopy(t *testing.T) {
	t.Parallel()
	fa := &fakeAI{}
	e := newTestEnv(t, func(s *envSettings) { s.ai = fa })
	ctv := e.register(t, "CTV mạng yếu")
	full := bigPhoto(t, 9800)
	small := shrink(t, full)
	if len(small)*2 > len(full) {
		t.Fatalf("bản nhỏ %d byte, gốc %d byte", len(small), len(full))
	}
	sum := sha256.Sum256(full)
	sha := hex.EncodeToString(sum[:])
	meta := map[string]string{"meta": cameraMeta(1, 10.7), "sha": sha}
	expectStatus(t, e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{small}, meta), http.StatusOK)
	e.house.Wait()
	fa.mu.Lock()
	after := fa.calls
	fa.mu.Unlock()
	if after != 1 {
		t.Fatalf("kiểm bản nhỏ: AI gọi %d lần", after)
	}
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", [][]byte{full}, map[string]string{"meta": cameraMeta(1, 10.7)})
	expectStatus(t, r, http.StatusCreated)
	e.house.Wait()
	fa.mu.Lock()
	calls := fa.calls
	fa.mu.Unlock()
	if calls != 1 {
		t.Fatalf("nộp ảnh gốc đã kiểm bằng bản nhỏ mà AI đọc lại: %d lần", calls)
	}
	r = e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{small}, meta)
	if !bytes.Contains(r.Raw, []byte("đã nộp ở hồ sơ")) {
		t.Fatalf("kiểm lại ảnh đã nộp (bản nhỏ + sha gốc) phải báo trùng: %s", r.Raw)
	}
}
