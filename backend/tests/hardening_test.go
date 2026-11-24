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

	"vuathau/backend/internal/integration/lark"
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

// Đợt 2: nhà thầu gửi lại nhật ký sau khi mất mạng (lần trước đã lưu xong mà app không nhận được trả lời):
// trả lại nhật ký cũ, không báo lỗi, không tạo nhật ký thứ 2. File trùng lẫn file mới: lưu file mới như thường.
func TestNhaThauLogRetry(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0908777003", "name": "Anh Sáu", "company": "Xây dựng Sáu"}), http.StatusOK)
	tok := ntLogin(t, e, "0908777003")
	r := e.do(t, http.MethodPost, "/api/v1/nt/projects", tok, map[string]string{"name": "Nhà phố Gò Vấp", "address": "Gò Vấp"})
	expectStatus(t, r, http.StatusCreated)
	var cp struct {
		Project struct {
			ID int64 `json:"id"`
		} `json:"project"`
	}
	decodeData(t, r, &cp)
	path := fmt.Sprintf("/api/v1/nt/projects/%d/logs", cp.Project.ID)
	files := [][]byte{photo(t, 7801), photo(t, 7802)}
	type logRes struct {
		Log struct {
			ID int64 `json:"id"`
		} `json:"log"`
		Photos int `json:"photos"`
	}
	var first, again logRes
	r = e.upload(t, path, tok, "files", files, map[string]string{"note": "Xây tường tầng 1"})
	expectStatus(t, r, http.StatusCreated)
	decodeData(t, r, &first)
	r = e.upload(t, path, tok, "files", files, map[string]string{"note": "Xây tường tầng 1"})
	expectStatus(t, r, http.StatusCreated)
	decodeData(t, r, &again)
	if again.Log.ID != first.Log.ID || again.Photos != 2 {
		t.Fatalf("gửi lại: lần 1 %+v, lần 2 %s", first, r.Raw)
	}
	var n int
	if err := e.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM nt_logs WHERE project_id = $1`, cp.Project.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("số nhật ký: %d %v", n, err)
	}
}

// Đợt 2: CTV tự xoá tài khoản rồi đồng bộ form Lark chạy lại: không tạo lại tài khoản cho số đó.
// Nhà thầu tự xoá được tài khoản (CH Play), trang công khai thôi hiện.
func TestDeletedAccountsStayDeleted(t *testing.T) {
	t.Parallel()
	fl := &fakeLark{}
	e := newTestEnv(t, func(s *envSettings) { s.lark = fl })
	ctx := context.Background()
	fl.recs = []lark.Record{larkRec(t, `{"record_id":"r1","fields":{"Họ và tên":"Lê Văn Xoá","Số điện thoại":"0985000111","Duyệt tham gia":true}}`)}
	if n, _, err := e.user.SyncLarkCTVs(ctx); err != nil || n != 1 {
		t.Fatalf("tạo CTV từ Lark: %d %v", n, err)
	}
	r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": "0985000111", "as": model.RoleCTV})
	expectStatus(t, r, http.StatusOK)
	var d struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, r, &d)
	expectStatus(t, e.do(t, http.MethodDelete, "/api/v1/me", d.AccessToken, nil), http.StatusOK)
	if n, _, err := e.user.SyncLarkCTVs(ctx); err != nil || n != 0 {
		t.Fatalf("Lark tạo lại CTV đã xoá: %d %v", n, err)
	}

	admin := e.staff(t, model.RoleAdmin)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0908777004", "name": "Anh Bảy", "company": "Xây dựng Bảy"}), http.StatusOK)
	nt := ntLogin(t, e, "0908777004")
	var pf struct {
		Code string `json:"code"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/portfolio", nt, nil), &pf)
	expectStatus(t, e.do(t, http.MethodDelete, "/api/v1/me", nt, nil), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", nt, nil), http.StatusUnauthorized)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/public/nt/"+pf.Code, "", nil), http.StatusNotFound)
}

// Đợt 2: tải lại trang hồ sơ năng lực nhiều lần chỉ tính 1 lượt xem.
func TestPortfolioViewsCountedOnce(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0908777005", "name": "Chị Tám", "company": "Xây dựng Tám"}), http.StatusOK)
	nt := ntLogin(t, e, "0908777005")
	var pf struct {
		Code string `json:"code"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/portfolio", nt, nil), &pf)
	for range 5 {
		expectStatus(t, e.do(t, http.MethodGet, "/api/v1/public/nt/"+pf.Code, "", nil), http.StatusOK)
	}
	var views int
	if err := e.db.QueryRow(context.Background(), `SELECT p.views FROM nt_profiles p JOIN users u ON u.id = p.user_id WHERE u.username = '0908777005'`).Scan(&views); err != nil || views != 1 {
		t.Fatalf("lượt xem: %d %v", views, err)
	}
}

// Đợt 2: đổi địa chỉ dự án mà không ghim lại, tra không ra toạ độ: bỏ ghim cũ (trước đây ghim cũ ở chỗ sai vẫn còn).
func TestProjectAddressChangeClearsPin(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0908777006", "name": "Anh Chín", "company": "Xây dựng Chín"}), http.StatusOK)
	nt := ntLogin(t, e, "0908777006")
	r := e.do(t, http.MethodPost, "/api/v1/nt/projects", nt, map[string]any{"name": "Nhà Thủ Đức", "address": "Thủ Đức", "lat": 10.85, "lng": 106.77})
	expectStatus(t, r, http.StatusCreated)
	var cp struct {
		Project struct {
			ID  int64    `json:"id"`
			Lat *float64 `json:"lat"`
		} `json:"project"`
	}
	decodeData(t, r, &cp)
	if cp.Project.Lat == nil {
		t.Fatalf("tạo dự án có ghim: %s", r.Raw)
	}
	expectStatus(t, e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/nt/projects/%d", cp.Project.ID), nt,
		map[string]any{"name": "Nhà Thủ Đức", "address": "Chỗ khác không tra được xyz"}), http.StatusOK)
	var lat *float64
	if err := e.db.QueryRow(context.Background(), `SELECT lat FROM nt_projects WHERE id = $1`, cp.Project.ID).Scan(&lat); err != nil || lat != nil {
		t.Fatalf("ghim cũ còn: %v %v", lat, err)
	}
}
