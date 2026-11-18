package tests

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

func fakeVideo(seed uint64, n int) []byte {
	rnd := rand.New(rand.NewPCG(seed, seed*3+1))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rnd.IntN(256))
	}
	return b
}

// uploadVideos: gửi multipart "files" với tên file + kiểu tuỳ ý.
func (e *testEnv) uploadVideos(t *testing.T, path, tok string, names []string, files [][]byte) apiResp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for i, f := range files {
		fw, err := w.CreateFormFile("files", names[i])
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(f)
	}
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.send(t, req)
}

func TestVideos(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV quay video")
	other := e.register(t, "CTV khác")
	admin := e.staff(t, model.RoleAdmin)
	sale := e.staff(t, model.RoleSale)

	// hồ sơ nháp
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", [][]byte{photo(t, 9100), photo(t, 9101)}, map[string]string{"meta": cameraMeta(2, 10.8)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	hid := *up.HouseID
	path := fmt.Sprintf("/api/v1/houses/%d/videos", hid)

	v1, v2 := fakeVideo(1, 300_000), fakeVideo(2, 120_000)
	r = e.uploadVideos(t, path, ctv.AccessToken, []string{"quay-1.mp4", "quay-2.mov", "ghi-chu.txt"}, [][]byte{v1, v2, []byte("x")})
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &up)
	if up.Added != 2 || len(up.Messages) != 1 || !strings.Contains(up.Messages[0], "không phải file video") {
		t.Fatalf("tải video: %s", r.Raw)
	}
	// video trùng: không thêm lần 2
	r = e.uploadVideos(t, path, ctv.AccessToken, []string{"lai.mp4"}, [][]byte{v1})
	decodeData(t, r, &up)
	if up.Added != 0 || len(up.Messages) != 1 {
		t.Fatalf("video trùng: %s", r.Raw)
	}
	// CTV khác không thêm / xem được
	r = e.uploadVideos(t, path, other.AccessToken, []string{"a.mp4"}, [][]byte{fakeVideo(3, 1000)})
	if r.Status == http.StatusOK {
		t.Fatalf("CTV khác thêm được video: %s", r.Raw)
	}

	var det service.HouseDetail
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), admin.AccessToken, nil), &det)
	if len(det.Videos) != 2 || det.Videos[1].Mime != "video/quicktime" || det.Videos[0].Size != int64(len(v1)) || det.Valid != 2 {
		t.Fatalf("chi tiết: %+v valid=%d", det.Videos, det.Valid)
	}
	src := det.Videos[0].Src

	// link ký: xem được không cần đăng nhập, tua được (Range)
	g := e.getRaw(t, src, "")
	if g.Status != http.StatusOK || !bytes.Equal(g.Raw, v1) {
		t.Fatalf("xem video: %d, %d byte", g.Status, len(g.Raw))
	}
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+src, nil)
	req.Header.Set("Range", "bytes=100-199")
	g = e.sendRaw(t, req)
	if g.Status != http.StatusPartialContent || !bytes.Equal(g.Raw, v1[100:200]) {
		t.Fatalf("tua video: %d, %d byte", g.Status, len(g.Raw))
	}
	// sửa chữ ký / đổi id / hết hạn: chặn
	for _, bad := range []string{
		src[:len(src)-1] + "0",
		strings.Replace(src, fmt.Sprintf("/videos/%d?", det.Videos[0].ID), fmt.Sprintf("/videos/%d?", det.Videos[1].ID), 1),
		fmt.Sprintf("/api/v1/videos/%d?e=%d&s=x", det.Videos[0].ID, time.Now().Add(-time.Hour).Unix()),
		fmt.Sprintf("/api/v1/videos/%d", det.Videos[0].ID),
	} {
		if g := e.getRaw(t, bad, ""); g.Status != http.StatusForbidden {
			t.Fatalf("link sai %s: %d", bad, g.Status)
		}
	}

	// xoá 1 video
	r = e.do(t, http.MethodDelete, fmt.Sprintf("%s/%d", path, det.Videos[1].ID), ctv.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), ctv.AccessToken, nil), &det)
	if len(det.Videos) != 1 {
		t.Fatalf("sau xoá: %d video", len(det.Videos))
	}
	// sale không gọi được API thêm video
	if r := e.uploadVideos(t, path, sale.AccessToken, []string{"a.mp4"}, [][]byte{fakeVideo(4, 1000)}); r.Status != http.StatusForbidden {
		t.Fatalf("sale thêm video: %d", r.Status)
	}
}

func TestInboxVideos(t *testing.T) {
	t.Parallel()
	src := &fakeInbox{images: map[string][]byte{}, videos: map[string][]string{}}
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src })
	admin := e.staff(t, model.RoleAdmin)
	a := inbox.Contact{ID: "44444444-aaaa-4bbb-8ccc-000000000004", Name: "Nguyễn Văn V", Phone: "+84907777777"}

	// hồ sơ có sẵn trước khi có phần video (chưa có video)
	src.add(t, "log-v1", a, 7000, 5, site("1 Trần Phú, Nha Trang", "0901112223", "12.238791°N, 109.196749°E"))
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil), http.StatusOK)
	ctx := context.Background()
	idOf := func(ext string) int64 {
		var id int64
		if err := e.db.QueryRow(ctx, `SELECT id FROM houses WHERE ext_id = $1`, ext).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	videos := func(id int64) []model.Video {
		var det service.HouseDetail
		decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", id), admin.AccessToken, nil), &det)
		return det.Videos
	}
	if n := len(videos(idOf("log-v1"))); n != 0 {
		t.Fatalf("chưa có video: %d", n)
	}

	// dezon-inbox có video cho log-v1 (hồ sơ cũ) + log mới kèm video
	src.mu.Lock()
	src.images["media/v1.mp4"] = fakeVideo(11, 50_000)
	src.images["media/v2.mp4"] = fakeVideo(12, 60_000)
	src.images["media/v3.mp4"] = fakeVideo(13, 70_000)
	src.videos["log-v1"] = []string{"media/v1.mp4", "media/khong-co.mp4"} // 1 file hỏng: bỏ qua, không chặn
	src.mu.Unlock()
	src.add(t, "log-v2", a, 7100, 5, site("9 Lý Thường Kiệt, Huế", "0901112224", "16.463713°N, 107.590866°E"))
	src.mu.Lock()
	src.logs[len(src.logs)-1].Videos = []string{"media/v2.mp4", "media/v3.mp4"}
	src.mu.Unlock()
	// lần đồng bộ trước đã đánh dấu kéo video xong (lúc đó chưa có): xoá để chạy lại như khi mới nâng cấp
	if _, err := e.db.Exec(ctx, `DELETE FROM sync_state WHERE k = 'inbox_videos_v1'`); err != nil {
		t.Fatal(err)
	}
	r := e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
	var st service.SyncStatus
	decodeData(t, r, &st)
	if st.Error != "" || st.Added != 1 {
		t.Fatalf("sync: %s", r.Raw)
	}
	if n := len(videos(idOf("log-v1"))); n != 1 {
		t.Fatalf("hồ sơ cũ phải có 1 video: %d", n)
	}
	vs := videos(idOf("log-v2"))
	if len(vs) != 2 || vs[0].Size != 60_000 {
		t.Fatalf("hồ sơ mới: %+v", vs)
	}
	if g := e.getRaw(t, vs[1].Src, ""); g.Status != http.StatusOK || len(g.Raw) != 70_000 {
		t.Fatalf("xem video Messenger: %d", g.Status)
	}
	// đồng bộ lại: không kéo trùng
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil), http.StatusOK)
	if n := len(videos(idOf("log-v1"))); n != 1 {
		t.Fatalf("đồng bộ lại kéo trùng: %d", n)
	}

}
