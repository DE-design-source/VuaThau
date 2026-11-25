package config

import (
	"os"
	"path/filepath"
	"testing"
)

// .env chung ở gốc repo (../.env khi chạy trong backend/) được nạp; .env cạnh thư mục chạy (backend/.env cũ) ưu tiên
// hơn; biến môi trường thật ưu tiên hơn cả hai.
func TestLoadDotenvRoot(t *testing.T) {
	root := t.TempDir()
	be := filepath.Join(root, "backend")
	if err := os.Mkdir(be, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, ".env"), "VT_TEST_ROOT=goc\nVT_TEST_BOTH=goc\nVT_TEST_ENV=goc\n")
	write(filepath.Join(be, ".env"), "VT_TEST_BOTH=backend\n")
	t.Chdir(be)
	t.Setenv("VT_TEST_ENV", "moi-truong")
	for _, k := range []string{"VT_TEST_ROOT", "VT_TEST_BOTH"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	LoadDotenv()
	for k, want := range map[string]string{"VT_TEST_ROOT": "goc", "VT_TEST_BOTH": "backend", "VT_TEST_ENV": "moi-truong"} {
		if got := os.Getenv(k); got != want {
			t.Fatalf("%s = %q, muốn %q", k, got, want)
		}
	}
}
