// Package storage: lưu file ảnh trên đĩa. Ảnh hồ sơ: <DATA_DIR>/anh/<sha256> (gốc) và <sha256>_s.jpg (thu nhỏ);
// biên lai: <DATA_DIR>/bienlai/<tên>. SAO LƯU DATA_DIR cùng Postgres.
package storage

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"vuathau/backend/internal/pkg/imaging"
)

type Files struct {
	photos, receipts string
}

func New(dataDir string) (*Files, error) {
	f := &Files{photos: filepath.Join(dataDir, "anh"), receipts: filepath.Join(dataDir, "bienlai")}
	for _, d := range []string{f.photos, f.receipts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("tạo thư mục %s: %w", d, err)
		}
	}
	return f, nil
}

var (
	shaRE     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	receiptRE = regexp.MustCompile(`^[0-9]+-[0-9a-f]{8}\.jpg$`)
)

func (f *Files) photoPath(sha string, small bool) (string, error) {
	if !shaRE.MatchString(sha) {
		return "", fmt.Errorf("sha256 không hợp lệ")
	}
	if small {
		return filepath.Join(f.photos, sha+"_s.jpg"), nil
	}
	return filepath.Join(f.photos, sha), nil
}

// SmallSize: cạnh dài ảnh thu nhỏ (xem trên web / app, gửi AI). Trùng service.SmallSide.
const SmallSize = 1568

// SavePhoto ghi ảnh gốc và ảnh thu nhỏ. small = nil: chưa tạo, ReadPhoto tạo khi cần lần đầu
// (nộp ảnh không phải chờ nén lại: CTV nộp nhanh hơn, máy chủ đỡ tải lúc đông người).
func (f *Files) SavePhoto(sha string, original, small []byte) error {
	p, err := f.photoPath(sha, false)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, original, 0o644); err != nil {
		return err
	}
	if small == nil {
		return nil
	}
	s, _ := f.photoPath(sha, true)
	return os.WriteFile(s, small, 0o644)
}

func (f *Files) ReadPhoto(sha string, small bool) ([]byte, error) {
	p, err := f.photoPath(sha, small)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err == nil || !small || !os.IsNotExist(err) {
		return b, err
	}
	// chưa có ảnh thu nhỏ: tạo từ ảnh gốc rồi lưu cho lần sau
	src, err := f.ReadPhoto(sha, false)
	if err != nil {
		return nil, err
	}
	img, _, _, err := imaging.Open(src)
	if err != nil {
		return src, nil // không đọc được: trả ảnh gốc
	}
	if b, err = imaging.SmallJPEG(img, SmallSize); err != nil {
		return src, nil
	}
	tmp := p + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, p) != nil {
		_ = os.Remove(tmp)
	}
	return b, nil
}

func (f *Files) DeletePhoto(sha string) {
	for _, small := range []bool{false, true} {
		if p, err := f.photoPath(sha, small); err == nil {
			_ = os.Remove(p)
		}
	}
	if shaRE.MatchString(sha) {
		_ = os.Remove(filepath.Join(f.photos, sha+"_c.jpg"))
	}
	f.DeleteMasked(sha)
}

// ReadMasked: ảnh đã che SĐT (vùng boxes) cỡ thu nhỏ (card=false) hoặc cỡ thẻ. Lưu <sha>_m.jpg / <sha>_mc.jpg cho lần
// sau; vùng che đổi (quét lại) thì gọi DeleteMasked.
func (f *Files) ReadMasked(sha string, card bool, boxes []imaging.RelBox) ([]byte, error) {
	if !shaRE.MatchString(sha) {
		return nil, fmt.Errorf("sha256 không hợp lệ")
	}
	name := sha + "_m.jpg"
	if card {
		name = sha + "_mc.jpg"
	}
	p := filepath.Join(f.photos, name)
	if b, err := os.ReadFile(p); err == nil {
		return b, nil
	}
	var src []byte
	var err error
	if card {
		src, err = f.ReadCard(sha)
	} else {
		src, err = f.ReadPhoto(sha, true)
	}
	if err != nil {
		return nil, err
	}
	b, err := imaging.MaskJPEG(src, boxes)
	if err != nil {
		return nil, err // không che được: không trả ảnh chưa che
	}
	tmp := p + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, p) != nil {
		_ = os.Remove(tmp)
	}
	return b, nil
}

// DeleteMasked: xoá ảnh đã che (để tạo lại theo vùng che mới).
func (f *Files) DeleteMasked(sha string) {
	if shaRE.MatchString(sha) {
		_ = os.Remove(filepath.Join(f.photos, sha+"_m.jpg"))
		_ = os.Remove(filepath.Join(f.photos, sha+"_mc.jpg"))
	}
}

// CardSize: cạnh dài ảnh cỡ thẻ (lưới thẻ công trình trên web).
const CardSize = 640

// ReadCard: ảnh cỡ thẻ (cạnh dài <= CardSize). Tạo lần đầu từ ảnh thu nhỏ rồi lưu <sha>_c.jpg cho lần sau.
func (f *Files) ReadCard(sha string) ([]byte, error) {
	if !shaRE.MatchString(sha) {
		return nil, fmt.Errorf("sha256 không hợp lệ")
	}
	p := filepath.Join(f.photos, sha+"_c.jpg")
	if b, err := os.ReadFile(p); err == nil {
		return b, nil
	}
	src, err := f.ReadPhoto(sha, true)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return src, nil // không đọc được: trả ảnh thu nhỏ
	}
	b, err := imaging.SmallJPEG(img, CardSize)
	if err != nil {
		return src, nil
	}
	// ghi file tạm rồi đổi tên: người khác không đọc phải file đang ghi dở
	tmp := p + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if os.WriteFile(tmp, b, 0o644) == nil {
		if os.Rename(tmp, p) != nil {
			_ = os.Remove(tmp)
		}
	}
	return b, nil
}

func (f *Files) SaveReceipt(name string, data []byte) error {
	if !receiptRE.MatchString(name) {
		return fmt.Errorf("tên biên lai không hợp lệ")
	}
	return os.WriteFile(filepath.Join(f.receipts, name), data, 0o644)
}

func (f *Files) ReadReceipt(name string) ([]byte, error) {
	if !receiptRE.MatchString(name) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(f.receipts, name))
}

func (f *Files) DeleteReceipt(name string) {
	if receiptRE.MatchString(name) {
		_ = os.Remove(filepath.Join(f.receipts, name))
	}
}
