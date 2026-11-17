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

// SavePhoto ghi ảnh gốc và ảnh thu nhỏ.
func (f *Files) SavePhoto(sha string, original, small []byte) error {
	p, err := f.photoPath(sha, false)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, original, 0o644); err != nil {
		return err
	}
	s, _ := f.photoPath(sha, true)
	return os.WriteFile(s, small, 0o644)
}

func (f *Files) ReadPhoto(sha string, small bool) ([]byte, error) {
	p, err := f.photoPath(sha, small)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
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
