// Package storage: lưu file ảnh trên đĩa. Ảnh hồ sơ: <DATA_DIR>/anh/<sha256> (gốc) và <sha256>_s.jpg (thu nhỏ);
// biên lai: <DATA_DIR>/bienlai/<tên>. SAO LƯU DATA_DIR cùng Postgres.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
