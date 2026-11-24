package tests

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"net/http"
	"testing"

	"vuathau/backend/internal/integration/ocr"
	"vuathau/backend/internal/model"
)

// fakeScanOCR: như fakeOCR (1 SĐT góc trên trái) + 1 dòng chữ bảng hiệu (giữa ảnh).
type fakeScanOCR struct{ fakeOCR }

func (f *fakeScanOCR) Scan(ctx context.Context, b []byte, known []string) ([]ocr.Box, []ocr.Box, error) {
	phones, err := f.Phones(ctx, b, known)
	return phones, []ocr.Box{{X: 0.3, Y: 0.45, W: 0.4, H: 0.1}}, err
}

// Web bật "Ẩn thông tin CTV, nhà thầu" (?hide=1): ảnh chưa quét chữ thì làm mờ cả ảnh; quét rồi thì che SĐT + mọi
// dòng chữ (khác bản chỉ che SĐT); cỡ thẻ vẫn đúng cỡ.
func TestPhotoHideText(t *testing.T) {
	t.Parallel()
	fo := &fakeScanOCR{}
	e := newTestEnv(t, func(s *envSettings) { s.ocr = fo })
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV ảnh có bảng hiệu")
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", [][]byte{bigPhoto(t, 9600)}, map[string]string{"meta": cameraMeta(1, 10.6)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	var pid int64
	_ = e.db.QueryRow(context.Background(), `SELECT min(id) FROM photos WHERE house_id = $1`, *up.HouseID).Scan(&pid)
	path := fmt.Sprintf("/api/v1/photos/%d", pid)
	get := func(tok, q string) []byte {
		t.Helper()
		g := e.getRaw(t, path+q, tok)
		if g.Status != http.StatusOK {
			t.Fatalf("GET %s%s: %d", path, q, g.Status)
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(g.Raw)); err != nil {
			t.Fatalf("GET %s%s: không phải ảnh: %v", path, q, err)
		}
		return g.Raw
	}
	orig := get(admin.AccessToken, "")
	blurred := get(admin.AccessToken, "?hide=1")
	if bytes.Equal(blurred, orig) {
		t.Fatal("chưa quét chữ mà ?hide=1 trả ảnh gốc")
	}
	// CTV xem: quét (SĐT + chữ) rồi lưu
	phoneOnly := get(ctv.AccessToken, "")
	hidden := get(admin.AccessToken, "?hide=1")
	if bytes.Equal(hidden, orig) || bytes.Equal(hidden, blurred) || bytes.Equal(hidden, phoneOnly) {
		t.Fatal("ảnh ẩn thông tin phải che cả chữ (khác gốc, khác bản mờ, khác bản chỉ che SĐT)")
	}
	if !bytes.Equal(get(ctv.AccessToken, "?hide=1"), hidden) {
		t.Fatal("CTV bật ẩn thông tin phải thấy cùng bản")
	}
	card := get(admin.AccessToken, "?hide=1&size=card")
	if c, _, _ := image.DecodeConfig(bytes.NewReader(card)); max(c.Width, c.Height) != 640 {
		t.Fatalf("ảnh thẻ ẩn thông tin: %dx%d", c.Width, c.Height)
	}
	var text []byte
	_ = e.db.QueryRow(context.Background(), `SELECT text_mask FROM photos WHERE id = $1`, pid).Scan(&text)
	if len(text) < 10 {
		t.Fatalf("chưa lưu vùng chữ: %s", text)
	}
}
