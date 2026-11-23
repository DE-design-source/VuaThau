package tests

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
)

// Nhà thầu trên app (đợt 1, 23/11/2026): admin thêm nhà thầu -> nhà thầu vào app bằng SĐT, tạo dự án, chụp nhật ký
// (ảnh / video + mô tả + giờ + GPS), xem lại; nhà thầu khác, CTV không xem / sửa được dự án của người khác.
func TestNhaThau(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV không phải nhà thầu")

	add := func(phone, name, company string) {
		t.Helper()
		r := e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken, map[string]string{"phone": phone, "name": name, "company": company})
		expectStatus(t, r, http.StatusOK)
	}
	login := func(phone string) string {
		t.Helper()
		r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone})
		expectStatus(t, r, http.StatusOK)
		var d struct {
			Registered  bool       `json:"registered"`
			AccessToken string     `json:"access_token"`
			User        model.User `json:"user"`
		}
		decodeData(t, r, &d)
		if !d.Registered || d.User.Role != model.RoleNhaThau {
			t.Fatalf("nhà thầu đăng nhập: %s", r.Raw)
		}
		return d.AccessToken
	}
	add("0908111222", "Anh Hùng", "Công ty TNHH Xây dựng Hùng Phát")
	add("0908333444", "Chị Lan", "Xây dựng Lan Anh")
	// trùng SĐT
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken, map[string]string{"phone": "0908111222", "name": "X"}), http.StatusConflict)
	tok, other := login("0908111222"), login("0908333444")
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", tok, nil), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/config", tok, nil), http.StatusOK)

	// trang chủ: hồ sơ công ty
	var home struct {
		Profile struct {
			Company string `json:"company"`
		} `json:"profile"`
		Stats struct {
			Projects int `json:"projects"`
		} `json:"stats"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", tok, nil), &home)
	if home.Profile.Company != "Công ty TNHH Xây dựng Hùng Phát" || home.Stats.Projects != 0 {
		t.Fatalf("trang chủ: %+v", home)
	}

	// tạo dự án
	r := e.do(t, http.MethodPost, "/api/v1/nt/projects", tok, map[string]string{"name": "Biệt thự Bình Chánh", "address": "Đường 15C, Bình Chánh", "kind": "Biệt thự"})
	expectStatus(t, r, http.StatusCreated)
	var cp struct {
		Project struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"project"`
	}
	decodeData(t, r, &cp)
	pid := cp.Project.ID
	if pid == 0 || cp.Project.Status != "dang_thi_cong" {
		t.Fatalf("tạo dự án: %s", r.Raw)
	}
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/nt/projects", tok, map[string]string{"name": " "}), http.StatusBadRequest)

	// nhật ký: ô chữ trước, rồi 2 ảnh + 1 video (như app gửi)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("note", "Lắp cốp pha cột tầng 2")
	_ = w.WriteField("meta", `{"t":"2026-11-23 08:19","lat":10.78,"lng":106.71,"addr":"23 Trần Bạch Đằng"}`)
	for i, b := range [][]byte{photo(t, 7701), photo(t, 7702)} {
		fw, _ := w.CreateFormFile("files", fmt.Sprintf("anh-%d.jpg", i))
		_, _ = fw.Write(b)
	}
	fw, _ := w.CreateFormFile("files", "video-1.mp4")
	_, _ = fw.Write(bytes.Repeat([]byte("v"), 2048))
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/nt/projects/%d/logs", e.srv.URL, pid), &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	r = e.send(t, req)
	expectStatus(t, r, http.StatusCreated)
	var lr struct {
		Photos int `json:"photos"`
		Videos int `json:"videos"`
	}
	decodeData(t, r, &lr)
	if lr.Photos != 2 || lr.Videos != 1 {
		t.Fatalf("nhật ký: %s", r.Raw)
	}
	// nhật ký thứ 2: file trước, ô chữ sau (vẫn nhận mô tả); ảnh trùng bị bỏ qua
	r = e.upload(t, fmt.Sprintf("/api/v1/nt/projects/%d/logs", pid), tok, "files", [][]byte{photo(t, 7703), photo(t, 7701)},
		map[string]string{"note": "Đổ bê tông sàn tầng 1"})
	expectStatus(t, r, http.StatusCreated)

	// chi tiết dự án
	var det struct {
		Project struct {
			Photos int `json:"photos"`
			Videos int `json:"videos"`
			Logs   int `json:"logs"`
		} `json:"project"`
		Logs []struct {
			Note  string `json:"note"`
			Lat   *float64
			Media []struct {
				ID   int64  `json:"id"`
				Kind string `json:"kind"`
				Src  string `json:"src"`
			} `json:"media"`
		} `json:"logs"`
		Days int `json:"days"`
	}
	path := fmt.Sprintf("/api/v1/nt/projects/%d", pid)
	decodeData(t, e.do(t, http.MethodGet, path, tok, nil), &det)
	if det.Project.Photos != 3 || det.Project.Videos != 1 || det.Project.Logs != 2 || len(det.Logs) != 2 || det.Days < 1 {
		t.Fatalf("chi tiết: %+v", det)
	}
	notes := map[string]bool{}
	var photoID int64
	var videoSrc string
	for _, l := range det.Logs {
		notes[l.Note] = true
		for _, m := range l.Media {
			if m.Kind == "photo" && photoID == 0 {
				photoID = m.ID
			}
			if m.Kind == "video" {
				videoSrc = m.Src
			}
		}
	}
	if !notes["Lắp cốp pha cột tầng 2"] || !notes["Đổ bê tông sàn tầng 1"] || photoID == 0 || videoSrc == "" {
		t.Fatalf("nhật ký / ảnh: %+v", det.Logs)
	}
	// ảnh: chủ dự án + admin xem được; nhà thầu khác, CTV không
	pp := fmt.Sprintf("/api/v1/nt/photos/%d?size=small", photoID)
	if g := e.getRaw(t, pp, tok); g.Status != 200 || len(g.Raw) == 0 {
		t.Fatalf("ảnh nhật ký: %d", g.Status)
	}
	if g := e.getRaw(t, pp, admin.AccessToken); g.Status != 200 {
		t.Fatalf("admin xem ảnh: %d", g.Status)
	}
	if g := e.getRaw(t, pp, other); g.Status == 200 {
		t.Fatal("nhà thầu khác xem được ảnh")
	}
	if g := e.getRaw(t, videoSrc, ""); g.Status != 200 {
		t.Fatalf("video có chữ ký: %d", g.Status)
	}
	expectStatus(t, e.do(t, http.MethodGet, path, other, nil), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodPut, path, other, map[string]any{"name": "Cướp"}), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/nt/home", ctv.AccessToken, nil), http.StatusForbidden)

	// hoàn thành: tiến độ 100%
	r = e.do(t, http.MethodPut, path, tok, map[string]any{"name": "Biệt thự Bình Chánh", "address": "Đường 15C", "status": "hoan_thanh"})
	expectStatus(t, r, http.StatusOK)
	var up struct {
		Project struct {
			Status   string `json:"status"`
			Progress int    `json:"progress"`
		} `json:"project"`
	}
	decodeData(t, r, &up)
	if up.Project.Status != "hoan_thanh" || up.Project.Progress != 100 {
		t.Fatalf("hoàn thành: %s", r.Raw)
	}
	var lst struct {
		Rows []struct{ ID int64 } `json:"rows"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/projects?status=dang_thi_cong", tok, nil), &lst)
	if len(lst.Rows) != 0 {
		t.Fatal("lọc đang thi công sai")
	}

	// admin: danh sách nhà thầu
	var al struct {
		Rows []struct {
			Phone    string `json:"phone"`
			Projects int    `json:"projects"`
			Logs     int    `json:"logs"`
		} `json:"rows"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/admin/nt", admin.AccessToken, nil), &al)
	found := false
	for _, x := range al.Rows {
		if x.Phone == "0908111222" && x.Projects == 1 && x.Logs == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin danh sách nhà thầu: %+v", al.Rows)
	}
}
