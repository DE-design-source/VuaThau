package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ErrTooBig: file vượt giới hạn.
var ErrTooBig = errors.New("file quá lớn")

func (f *Files) videoDir() string { return filepath.Join(filepath.Dir(f.photos), "video") }

// SaveVideo chép r vào <DATA_DIR>/video/<sha256> (ghi file tạm rồi đổi tên), tối đa limit byte.
// Không đọc cả video vào RAM.
func (f *Files) SaveVideo(r io.Reader, limit int64) (sha string, size int64, err error) {
	dir := f.videoDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(dir, "up-*.tmp")
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, limit+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, err
	}
	if n > limit {
		return "", 0, ErrTooBig
	}
	if n == 0 {
		return "", 0, errors.New("file rỗng")
	}
	sha = hex.EncodeToString(h.Sum(nil))
	p := filepath.Join(dir, sha)
	if _, e := os.Stat(p); e == nil {
		_ = os.Remove(tmp.Name()) // đã có file y hệt
		return sha, n, nil
	}
	if err = os.Rename(tmp.Name(), p); err != nil {
		return "", 0, err
	}
	return sha, n, nil
}

// VideoPath: đường dẫn file video theo sha256.
func (f *Files) VideoPath(sha string) (string, error) {
	if !shaRE.MatchString(sha) {
		return "", errors.New("sha256 không hợp lệ")
	}
	return filepath.Join(f.videoDir(), sha), nil
}

func (f *Files) DeleteVideo(sha string) {
	if p, err := f.VideoPath(sha); err == nil {
		_ = os.Remove(p)
	}
}

// CleanVideoTemp: xoá file tải dở cũ hơn 1 ngày (máy chủ tắt giữa chừng).
func (f *Files) CleanVideoTemp() {
	ms, _ := filepath.Glob(filepath.Join(f.videoDir(), "up-*.tmp"))
	for _, m := range ms {
		if st, err := os.Stat(m); err == nil && time.Since(st.ModTime()) > 24*time.Hour {
			_ = os.Remove(m)
		}
	}
}
