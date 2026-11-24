package tests

import (
	"bytes"
	"encoding/json"
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

// Form "Đăng tải dự án mới" (thiết kế 24/11/2026): tỉnh + phường đúng danh sách 2025, ảnh dự án (ảnh đầu = ảnh bìa, tối đa 20),
// và báo cáo gửi chủ nhà: link công khai /bc/<mã> chỉ xem được đúng nhật ký đó.
func TestNhaThauFormVaBaoCao(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken,
		map[string]string{"phone": "0907555666", "name": "Anh Hưng", "company": "Smartcons"}), http.StatusOK)
	r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": "0907555666"})
	var lg struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, r, &lg)
	tok := lg.AccessToken

	// danh sách tỉnh / phường (công khai)
	var divs []struct {
		Name  string   `json:"name"`
		Wards []string `json:"wards"`
	}
	hr, err := http.Get(e.srv.URL + "/api/v1/geo/divisions")
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewDecoder(hr.Body).Decode(&divs)
	_ = hr.Body.Close()
	if err != nil || len(divs) != 34 {
		t.Fatalf("tỉnh: %d", len(divs))
	}

	// phường không thuộc tỉnh -> 400; thiếu tỉnh -> 400
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/nt/projects", tok,
		map[string]string{"name": "Nhà phố anh Hưng", "province": "TP. Hồ Chí Minh", "ward": "Phường Ba Đình"}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/nt/projects", tok,
		map[string]string{"name": "Nhà phố anh Hưng", "province": "Không có", "ward": "Phường An Khánh"}), http.StatusBadRequest)

	// đúng: tên tỉnh đầy đủ cũng nhận, địa chỉ ghép "số nhà, phường, tỉnh"
	r = e.do(t, http.MethodPost, "/api/v1/nt/projects", tok, map[string]any{"name": "Nhà phố anh Hưng", "province": "Thành phố Hồ Chí Minh",
		"ward": "phường an khánh", "street": "20 Nguyễn Thiện Thành", "kind": "Nhà phố", "lat": 10.78, "lng": 106.72})
	expectStatus(t, r, http.StatusCreated)
	var cp struct {
		Project struct {
			ID       int64  `json:"id"`
			Address  string `json:"address"`
			Province string `json:"province"`
			Ward     string `json:"ward"`
		} `json:"project"`
	}
	decodeData(t, r, &cp)
	pid := cp.Project.ID
	if cp.Project.Address != "20 Nguyễn Thiện Thành, Phường An Khánh, TP. Hồ Chí Minh" || cp.Project.Province != "TP. Hồ Chí Minh" || cp.Project.Ward != "Phường An Khánh" {
		t.Fatalf("địa chỉ: %s", r.Raw)
	}

	// ảnh dự án: 2 ảnh, ảnh đầu là ảnh bìa
	r = e.upload(t, fmt.Sprintf("/api/v1/nt/projects/%d/photos", pid), tok, "files", [][]byte{photo(t, 8801), photo(t, 8802)}, nil)
	expectStatus(t, r, http.StatusCreated)
	var det struct {
		Project struct {
			Cover *int64 `json:"cover"`
		} `json:"project"`
		Photos []struct {
			ID int64 `json:"id"`
		} `json:"photos"`
	}
	path := fmt.Sprintf("/api/v1/nt/projects/%d", pid)
	decodeData(t, e.do(t, http.MethodGet, path, tok, nil), &det)
	if len(det.Photos) != 2 || det.Project.Cover == nil || *det.Project.Cover != det.Photos[0].ID {
		t.Fatalf("ảnh dự án: %+v", det)
	}

	// nhật ký rồi báo cáo; ảnh dự án vẫn là ảnh bìa
	r = e.upload(t, fmt.Sprintf("/api/v1/nt/projects/%d/logs", pid), tok, "files", [][]byte{photo(t, 8803)}, map[string]string{"note": "Cốp pha. Lắp cốp pha cột tầng 2"})
	expectStatus(t, r, http.StatusCreated)
	var lr struct {
		Log struct {
			ID int64 `json:"id"`
		} `json:"log"`
	}
	decodeData(t, r, &lr)
	decodeData(t, e.do(t, http.MethodGet, path, tok, nil), &det)
	if *det.Project.Cover != det.Photos[0].ID {
		t.Fatalf("ảnh bìa đổi theo nhật ký: %+v", det)
	}
	var link struct {
		Code string `json:"code"`
		Path string `json:"path"`
	}
	decodeData(t, e.do(t, http.MethodGet, fmt.Sprintf("%s/logs/%d/report", path, lr.Log.ID), tok, nil), &link)
	if link.Code == "" || link.Path != "/bc/"+link.Code {
		t.Fatalf("link báo cáo: %+v", link)
	}
	// chủ nhà mở link (không đăng nhập)
	var rep struct {
		Company string   `json:"company"`
		Project string   `json:"project"`
		Note    string   `json:"note"`
		Photos  []string `json:"photos"`
	}
	rr := e.do(t, http.MethodGet, "/api/v1/public/bc/"+link.Code, "", nil)
	decodeData(t, rr, &rep)
	if rep.Company != "Smartcons" || rep.Project != "Nhà phố anh Hưng" || len(rep.Photos) != 1 || rep.Note == "" {
		t.Fatalf("báo cáo: %+v", rep)
	}
	if pr, err := http.Get(e.srv.URL + rep.Photos[0]); err != nil || pr.StatusCode != http.StatusOK || pr.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("ảnh báo cáo: %v %+v", err, pr)
	} else {
		_ = pr.Body.Close()
	}
	// ảnh dự án (không thuộc nhật ký) không xem được qua link báo cáo; mã sai -> 404
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/public/bc/%s/photos/%d", link.Code, det.Photos[0].ID), "", nil), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/public/bc/%d-000000000000", lr.Log.ID), "", nil), http.StatusNotFound)
	// nhà thầu khác không lấy được link báo cáo
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken, map[string]string{"phone": "0907555777", "name": "B"}), http.StatusOK)
	r = e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": "0907555777"})
	decodeData(t, r, &lg)
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("%s/logs/%d/report", path, lr.Log.ID), lg.AccessToken, nil), http.StatusNotFound)

	// nhật ký cũ hơn (giờ chụp 01/11) hiện ở "Nhật ký trước đây" của báo cáo, ảnh xem được qua link báo cáo; báo cáo có SĐT nhà thầu
	// ô chữ (meta có giờ chụp) gửi TRƯỚC ảnh, như app
	var mb bytes.Buffer
	mw := multipart.NewWriter(&mb)
	_ = mw.WriteField("note", "Đào móng")
	_ = mw.WriteField("meta", `{"t":"2026-11-01 08:00","lat":10.78,"lng":106.72}`)
	fw, _ := mw.CreateFormFile("files", "anh-1.jpg")
	_, _ = fw.Write(photo(t, 8804))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/nt/projects/%d/logs", e.srv.URL, pid), &mb)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	expectStatus(t, e.send(t, req), http.StatusCreated)
	var rep2 struct {
		Phone   string `json:"phone"`
		Logs    int    `json:"logs"`
		Earlier []struct {
			Note   string   `json:"note"`
			Photos []string `json:"photos"`
		} `json:"earlier"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/public/bc/"+link.Code, "", nil), &rep2)
	if rep2.Phone != "0907555666" || rep2.Logs != 2 || len(rep2.Earlier) != 1 || rep2.Earlier[0].Note != "Đào móng" || len(rep2.Earlier[0].Photos) != 1 {
		t.Fatalf("nhật ký trước đây: %+v", rep2)
	}
	if pr, err := http.Get(e.srv.URL + rep2.Earlier[0].Photos[0]); err != nil || pr.StatusCode != http.StatusOK {
		t.Fatalf("ảnh nhật ký trước đây: %v %+v", err, pr)
	} else {
		_ = pr.Body.Close()
	}
}

// Nhà thầu có tài khoản app thấy công trình CTV đã chụp có bảng hiệu ghi SĐT của mình (0xxx hay 84xxx đều khớp),
// không thấy công trình nhà thầu khác; ảnh bìa qua link công khai; không có CTV / tiền.
func TestNhaThauSites(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV chụp Duy Bình")
	phone, other := e.newPhone(), e.newPhone()
	h1 := e.newHouseP(t, ctv, 5, 9310, 10.70, phone)
	h2 := e.newHouseP(t, ctv, 5, 9410, 10.71, "+84"+phone[1:])
	e.newHouseP(t, ctv, 5, 9510, 10.72, other)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/admin/nt", admin.AccessToken, map[string]string{"phone": phone, "name": "Duy Bình"}), http.StatusOK)
	r := e.do(t, http.MethodPost, "/api/v1/auth/phone", "", map[string]string{"phone": phone, "as": "nhathau"})
	var lg struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, r, &lg)

	var sites struct {
		Rows []struct {
			ID       int64  `json:"id"`
			CoverURL string `json:"cover_url"`
			Photos   int    `json:"photos"`
		} `json:"rows"`
	}
	r = e.do(t, http.MethodGet, "/api/v1/nt/sites", lg.AccessToken, nil)
	decodeData(t, r, &sites)
	got := map[int64]bool{}
	for _, s := range sites.Rows {
		got[s.ID] = true
		if s.CoverURL == "" || s.Photos == 0 {
			t.Fatalf("công trình thiếu ảnh: %s", r.Raw)
		}
	}
	if len(sites.Rows) != 2 || !got[h1] || !got[h2] {
		t.Fatalf("công trình theo SĐT: %s", r.Raw)
	}
	if bytes.Contains(r.Raw, []byte("CTV chụp Duy Bình")) || bytes.Contains(r.Raw, []byte("amount")) {
		t.Fatalf("lộ CTV / tiền: %s", r.Raw)
	}
	if pr, err := http.Get(e.srv.URL + sites.Rows[0].CoverURL); err != nil || pr.StatusCode != http.StatusOK {
		t.Fatalf("ảnh bìa công khai: %v %+v", err, pr)
	} else {
		_ = pr.Body.Close()
	}
	var home struct {
		Stats struct {
			Sites int `json:"sites"`
		} `json:"stats"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/home", lg.AccessToken, nil), &home)
	if home.Stats.Sites != 2 {
		t.Fatalf("trang chủ: %+v", home)
	}
	// CTV không gọi được API nhà thầu
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/nt/sites", ctv.AccessToken, nil), http.StatusForbidden)

	// chụp nhật ký vào công trình đó: tạo 1 dự án gắn hồ sơ (địa chỉ, ảnh bìa từ hồ sơ CTV), bấm lại không tạo trùng
	claim := func(hid int64) apiResp {
		t.Helper()
		return e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/nt/sites/%d/claim", hid), lg.AccessToken, map[string]string{})
	}
	var cp struct {
		Project struct {
			ID       int64  `json:"id"`
			HouseID  *int64 `json:"house_id"`
			Address  string `json:"address"`
			CoverURL string `json:"cover_url"`
		} `json:"project"`
	}
	r = claim(h1)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &cp)
	pid := cp.Project.ID
	if pid == 0 || cp.Project.HouseID == nil || *cp.Project.HouseID != h1 || cp.Project.Address == "" || cp.Project.CoverURL == "" {
		t.Fatalf("nhận công trình: %s", r.Raw)
	}
	decodeData(t, claim(h1), &cp)
	if cp.Project.ID != pid {
		t.Fatalf("bấm lại tạo dự án mới: %d != %d", cp.Project.ID, pid)
	}
	// công trình không có SĐT nhà thầu này: không nhận được
	expectStatus(t, claim(h2+1000), http.StatusNotFound)
	// nhật ký vào dự án đó như dự án thường
	r = e.upload(t, fmt.Sprintf("/api/v1/nt/projects/%d/logs", pid), lg.AccessToken, "files", [][]byte{photo(t, 9610)}, map[string]string{"note": "Xây tô tầng 1"})
	expectStatus(t, r, http.StatusCreated)
	// đã thành dự án: không còn ở danh sách công trình ghi nhận, còn lại h2
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/nt/sites", lg.AccessToken, nil), &sites)
	if len(sites.Rows) != 1 || sites.Rows[0].ID != h2 {
		t.Fatalf("sau khi nhận: %+v", sites.Rows)
	}
}
