package tests

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"net/http"
	"sync/atomic"
	"testing"

	"vuathau/backend/internal/integration/ocr"
	"vuathau/backend/internal/model"
)

// fakeOCR: ảnh nào cũng có 1 SĐT ở góc trên trái (10%..40% ngang, 10%..20% dọc).
type fakeOCR struct{ calls atomic.Int64 }

func (*fakeOCR) Enabled() bool { return true }
func (f *fakeOCR) Phones(_ context.Context, b []byte, _ []string) ([]ocr.Box, error) {
	f.calls.Add(1)
	if _, _, err := image.DecodeConfig(bytes.NewReader(b)); err != nil {
		return nil, err
	}
	return []ocr.Box{{X: 0.1, Y: 0.1, W: 0.3, H: 0.1}}, nil
}

// Che SĐT trên ảnh (góp ý 20/11/2026): CTV (và sale, Drive) thấy ảnh đã che, admin / kế toán xem gốc; admin xem thử
// bản che bằng ?masked=1. Quét 1 lần rồi lưu.
func TestPhoneMask(t *testing.T) {
	t.Parallel()
	fo := &fakeOCR{}
	e := newTestEnv(t, func(s *envSettings) { s.ocr = fo })
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)
	ctv := e.register(t, "CTV che SĐT")

	files := [][]byte{}
	for i := range 6 {
		files = append(files, bigPhoto(t, 9500+uint64(i)))
	}
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(6, 10.5)})
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
		return g.Raw
	}
	orig := get(admin.AccessToken, "")
	if fo.calls.Load() != 0 {
		t.Fatalf("admin xem ảnh mà vẫn quét SĐT: %d", fo.calls.Load())
	}
	if !bytes.Equal(get(kt.AccessToken, ""), orig) {
		t.Fatal("kế toán phải thấy ảnh gốc")
	}
	masked := get(ctv.AccessToken, "")
	if bytes.Equal(masked, orig) {
		t.Fatal("CTV thấy ảnh chưa che SĐT")
	}
	if !bytes.Equal(get(admin.AccessToken, "?masked=1"), masked) {
		t.Fatal("admin xem thử bản che khác bản CTV thấy")
	}
	if !bytes.Equal(get(ctv.AccessToken, "?original=1"), masked) {
		t.Fatal("CTV lấy ?original=1 được ảnh chưa che")
	}
	card := get(ctv.AccessToken, "?size=card")
	if c, _, err := image.DecodeConfig(bytes.NewReader(card)); err != nil || max(c.Width, c.Height) != 640 {
		t.Fatalf("ảnh thẻ đã che: %v %dx%d", err, c.Width, c.Height)
	}
	if fo.calls.Load() != 1 {
		t.Fatalf("quét SĐT %d lần, cần 1 (lưu lại dùng tiếp)", fo.calls.Load())
	}

	// vùng che: điểm ảnh trong vùng gần như cùng màu (đã pixel hoá), ngoài vùng giữ nguyên
	mi, _, err := image.Decode(bytes.NewReader(masked))
	if err != nil {
		t.Fatal(err)
	}
	oi, _, _ := image.Decode(bytes.NewReader(orig))
	b := mi.Bounds()
	in := image.Pt(b.Dx()*25/100, b.Dy()*15/100)
	out := image.Pt(b.Dx()*70/100, b.Dy()*70/100)
	diff := func(p image.Point) int {
		r1, g1, b1, _ := mi.At(p.X, p.Y).RGBA()
		r2, g2, b2, _ := oi.At(p.X, p.Y).RGBA()
		d := func(a, b uint32) int { return max(int(a>>8)-int(b>>8), int(b>>8)-int(a>>8)) }
		return d(r1, r2) + d(g1, g2) + d(b1, b2)
	}
	if diff(in) < 20 {
		t.Fatalf("trong vùng che chưa đổi: %d", diff(in))
	}
	if diff(out) > 30 {
		t.Fatalf("ngoài vùng che bị đổi: %d", diff(out))
	}

	// admin thấy số SĐT đã che trên ảnh
	var d struct {
		Photos []struct {
			Masked int `json:"masked"`
		} `json:"photos"`
	}
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", *up.HouseID), admin.AccessToken, nil), &d)
	if len(d.Photos) == 0 || d.Photos[0].Masked != 1 {
		t.Fatalf("số SĐT đã che: %+v", d.Photos)
	}
}
