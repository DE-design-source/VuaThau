package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// App Android quét bảng hiệu trên khung ngắm: tên + SĐT nhà thầu gửi kèm ảnh (ct_name, ct_phone), điền khi hồ sơ chưa có.
func TestScannedContractorMeta(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV Quét")

	upload := func(seed uint64, name, phone string) int64 {
		t.Helper()
		var ms []map[string]any
		_ = json.Unmarshal([]byte(cameraMeta(5, 10.81)), &ms)
		ms[0]["ct_name"], ms[0]["ct_phone"] = name, phone
		b, _ := json.Marshal(ms)
		files := make([][]byte, 5)
		for i := range files {
			files[i] = photo(t, seed+uint64(i))
		}
		r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": string(b)})
		expectStatus(t, r, http.StatusCreated)
		var up uploadRes
		decodeData(t, r, &up)
		return *up.HouseID
	}
	get := func(id int64) (string, string) {
		var n, p string
		if err := e.db.QueryRow(context.Background(), `SELECT contractor_name, contractor_phone FROM houses WHERE id = $1`, id).Scan(&n, &p); err != nil {
			t.Fatal(err)
		}
		return n, p
	}

	id := upload(9800, "CÔNG TY TNHH TK XD MỘC MINH", "0906 909 901")
	if n, p := get(id); n != "CÔNG TY TNHH TK XD MỘC MINH" || p != "0906909901" {
		t.Fatalf("scanned: %q %q", n, p)
	}
	// SĐT sai dạng: bỏ, tên vẫn nhận
	id = upload(9900, "Xây dựng An Phát", "12345")
	if n, p := get(id); n != "Xây dựng An Phát" || p != "" {
		t.Fatalf("bad phone: %q %q", n, p)
	}
}
