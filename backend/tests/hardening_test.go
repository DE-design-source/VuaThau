package tests

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"strings"
	"sync"
	"testing"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// Rà soát 25/11/2026 (đợt 1): các lỗ đã vá.

// bombPNG: PNG thật 1×1 nhưng khai kích thước w×h ở IHDR (giải nén thật cần w*h*4 byte RAM).
func bombPNG(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// 8 byte chữ ký, 4 byte độ dài, "IHDR", rồi rộng / cao; CRC tính trên "IHDR" + 13 byte dữ liệu
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// Ảnh khai 50000×50000 điểm ảnh: trả lời "ảnh quá lớn", máy chủ không giải nén (trước đây: hết RAM, sập).
func TestImageBombRejected(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV gửi ảnh bom")
	bomb := bombPNG(t, 50000, 50000)

	r := e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{bomb}, nil)
	expectStatus(t, r, http.StatusOK)
	if !strings.Contains(string(r.Raw), "quá lớn") {
		t.Fatalf("kiểm ảnh bom: %s", r.Raw)
	}
	r = e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", [][]byte{photo(t, 9700), bomb}, map[string]string{"meta": cameraMeta(2, 10.8)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	if up.Added != 1 || !strings.Contains(string(r.Raw), "quá lớn") {
		t.Fatalf("nộp ảnh bom: %s", r.Raw)
	}
}

// Nhà thầu không tải được ảnh hồ sơ CTV qua /photos/{id} (dò id là lấy được ảnh mọi CTV).
func TestNhaThauCannotReadCTVPhotos(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV chụp")
	hid := e.newHouse(t, ctv, 5, 9710, 10.81)
	var det struct {
		Photos []struct {
			ID int64 `json:"id"`
		} `json:"photos"`
	}
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), ctv.AccessToken, nil), &det)
	if len(det.Photos) == 0 {
		t.Fatal("hồ sơ không có ảnh")
	}
	pid := det.Photos[0].ID
	expectStatus(t, e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", pid), ctv.AccessToken), http.StatusOK)
	expectStatus(t, e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", pid), admin.AccessToken), http.StatusOK)

	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0908777001", "name": "Anh Tư", "company": "Xây dựng Tư"}), http.StatusOK)
	nt := ntLogin(t, e, "0908777001")
	if r := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", pid), nt); r.Status != http.StatusForbidden {
		t.Fatalf("nhà thầu xem ảnh CTV: %d", r.Status)
	}
}

func ntLogin(t *testing.T, e *testEnv, phone string) string {
	t.Helper()
	r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone, "as": model.RoleNhaThau})
	expectStatus(t, r, http.StatusOK)
	var d struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, r, &d)
	if d.AccessToken == "" {
		t.Fatalf("nhà thầu đăng nhập: %s", r.Raw)
	}
	return d.AccessToken
}

// Nhà thầu bị khoá: hồ sơ năng lực công khai /nt/<mã> thôi hiện.
func TestLockedNhaThauPublicHidden(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0908777002", "name": "Chị Năm", "company": "Xây dựng Năm"}), http.StatusOK)
	nt := ntLogin(t, e, "0908777002")
	var pf struct {
		Code string `json:"code"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/portfolio", nt, nil), &pf)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/public/nt/"+pf.Code, "", nil), http.StatusOK)

	var me struct {
		User model.User `json:"user"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/me", nt, nil), &me)
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/lock", me.User.ID), admin.AccessToken,
		map[string]bool{"locked": true}), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/public/nt/"+pf.Code, "", nil), http.StatusNotFound)
}

// Tải nhiều đợt ảnh cùng lúc vào 1 hồ sơ: không vượt số ảnh tối đa (trước đây mỗi đợt đọc số ảnh rồi mới thêm).
func TestConcurrentUploadsRespectMax(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV tải song song")
	files := make([][]byte, 15)
	for i := range files {
		files[i] = photo(t, 9800+uint64(i))
	}
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(15, 10.83)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	hid := *up.HouseID

	var wg sync.WaitGroup
	for g := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch := make([][]byte, 5)
			for i := range batch {
				batch[i] = photo(t, 9900+uint64(g*10+i))
			}
			e.upload(t, fmt.Sprintf("/api/v1/houses/%d/photos", hid), ctv.AccessToken, "files", batch, map[string]string{"meta": cameraMeta(5, 10.83)})
		}()
	}
	wg.Wait()
	var n int
	if err := e.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM photos WHERE house_id = $1`, hid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > model.MaxPhotos {
		t.Fatalf("hồ sơ có %d ảnh, tối đa %d", n, model.MaxPhotos)
	}
}

// Hồ sơ đã gửi duyệt: lệnh ghi của CTV bị chặn ngay trong giao dịch (không chỉ ở bước kiểm trước).
func TestDraftWritesBlockedAfterSubmit(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV đã gửi")
	hid := e.newHouse(t, ctv, 5, 9950, 10.85)
	photos := repository.NewPhotoRepository(e.db)
	houses := repository.NewHouseRepository(e.db)
	ctx := context.Background()
	if err := photos.InsertDraft(ctx, &model.Photo{HouseID: hid, SHA256: strings.Repeat("ab", 32)}, model.MaxPhotos); err != repository.ErrNotEditable {
		t.Fatalf("thêm ảnh vào hồ sơ đã gửi: %v", err)
	}
	if err := houses.UpdateDraftFields(ctx, hid, model.HouseFields{Address: "sửa lén"}); err != repository.ErrNotEditable {
		t.Fatalf("sửa hồ sơ đã gửi: %v", err)
	}
	ps, err := photos.ListByHouse(ctx, hid)
	if err != nil || len(ps) == 0 {
		t.Fatalf("ảnh: %v", err)
	}
	if _, err := photos.DeleteDraft(ctx, ps[0].ID, hid); err != repository.ErrNotEditable {
		t.Fatalf("xoá ảnh hồ sơ đã gửi: %v", err)
	}
}

// Gửi duyệt: ảnh / video đổi sau lúc tính tiền thì không gửi (tiền phải theo đúng bộ ảnh được gửi).
func TestSubmitRejectsChangedMedia(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV gửi lúc đang xoá ảnh")
	files := [][]byte{photo(t, 9960), photo(t, 9961), photo(t, 9962)}
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(3, 10.86)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	hid := *up.HouseID
	photos := repository.NewPhotoRepository(e.db)
	houses := repository.NewHouseRepository(e.db)
	ctx := context.Background()
	ps, _ := photos.ListByHouse(ctx, hid)
	stale := repository.SigOf(ps, nil)
	if _, err := photos.DeleteDraft(ctx, ps[0].ID, hid); err != nil {
		t.Fatal(err)
	}
	if err := houses.SubmitDraft(ctx, hid, 1, nil, nil, "", false, stale); err != repository.ErrChanged {
		t.Fatalf("gửi với bộ ảnh cũ: %v", err)
	}
	ps, _ = photos.ListByHouse(ctx, hid)
	if err := houses.SubmitDraft(ctx, hid, 1, nil, nil, "", false, repository.SigOf(ps, nil)); err != nil {
		t.Fatalf("gửi với bộ ảnh đúng: %v", err)
	}
}
