package tests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/service"
)

// fakeInbox: dezon-inbox giả (project_logs + kho ảnh).
type fakeInbox struct {
	mu     sync.Mutex
	logs   []inbox.Log
	images map[string][]byte
	videos map[string][]string // mã hồ sơ -> khoá video (kéo video cho hồ sơ đã nhập)
}

func (f *fakeInbox) Logs(_ context.Context, at time.Time, id string, limit int) ([]inbox.Log, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []inbox.Log{}
	for _, l := range f.logs {
		if l.UpdatedAt.After(at) || (l.UpdatedAt.Equal(at) && l.ID > id) {
			out = append(out, l)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeInbox) Image(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.images[key]
	if !ok {
		return nil, fmt.Errorf("không có %s", key)
	}
	return b, nil
}

func (f *fakeInbox) Videos(_ context.Context, ids []string) (map[string][]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]string{}
	for _, id := range ids {
		if v := f.videos[id]; len(v) > 0 {
			out[id] = v
		}
	}
	return out, nil
}

func (f *fakeInbox) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.images[key]
	if !ok {
		return nil, "", fmt.Errorf("không có %s", key)
	}
	return io.NopCloser(bytes.NewReader(b)), "video/mp4", nil
}

func (f *fakeInbox) Close() {}

func (f *fakeInbox) add(t *testing.T, id string, c inbox.Contact, seed uint64, n int, fields map[string]string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	l := inbox.Log{ID: id, CreatedAt: time.Now(), UpdatedAt: time.Now().Add(time.Duration(len(f.logs)) * time.Millisecond),
		Fields: fields, Contact: c}
	for i := range n {
		k := fmt.Sprintf("media/%s-%d.jpg", id, i)
		f.images[k] = photo(t, seed+uint64(i))
		l.Images = append(l.Images, inbox.Image{Key: k, Mime: "image/jpeg", SentAt: time.Now()})
	}
	f.logs = append(f.logs, l)
}

func site(addr, phone, coord string) map[string]string {
	return map[string]string{"dia_chi": addr, "nha_thau": "CÔNG TY XÂY DỰNG HÀM RỒNG", "sdt_nha_thau": phone,
		"ngay_chup": "16/11/2026", "toa_do": coord, "loai_cong_trinh": "Nhà phố", "tien_do": "Hoàn thiện cơ bản"}
}

func TestInboxSync(t *testing.T) {
	t.Parallel()
	src := &fakeInbox{images: map[string][]byte{}}
	e := newTestEnv(t, func(s *envSettings) { s.inbox = src })
	admin := e.staff(t, model.RoleAdmin)
	appCTV := e.register(t, "CTV dùng app")

	a := inbox.Contact{ID: "11111111-aaaa-4bbb-8ccc-000000000001", Name: "Trần Văn A", Phone: "+84901111111",
		Profile: map[string]string{"so_tai_khoan": "Vietcombank 0123456789 TRAN VAN A"}}
	b := inbox.Contact{ID: "22222222-aaaa-4bbb-8ccc-000000000002", Name: "Lê Thị B"} // chưa để lại SĐT
	c := inbox.Contact{ID: "33333333-aaaa-4bbb-8ccc-000000000003", Name: "Facebook C", Phone: appCTV.User.Username}

	src.add(t, "log-1", a, 5000, 6, site("395 Bùi Thị Xuân, Bình Dương", "0943.488.882", "11.070510°N, 106.682880°E"))
	// cách log-1 khoảng 20m, cùng SĐT nhà thầu: trùng công trình -> tự từ chối
	src.add(t, "log-2", b, 5100, 5, site("395 Bùi Thị Xuân", "0943488882", "11.070690°N, 106.682880°E"))
	src.add(t, "log-3", a, 5200, 5, site("P. Trường Vinh, Nghệ An", "0985329298", "18.660076°N, 105.685129°E"))
	src.add(t, "log-4", c, 5300, 5, site("12 Lê Lợi, Huế", "0911222333", "16.463713°N, 107.590866°E"))
	src.add(t, "log-5", a, 5400, 0, site("không có ảnh", "", "")) // không có ảnh: bỏ qua

	r := e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var st service.SyncStatus
	decodeData(t, r, &st)
	if !st.Enabled || st.Added != 4 || st.Rejected != 1 || st.Total != 4 || st.Error != "" {
		t.Fatalf("sync: %s", r.Raw)
	}
	// đồng bộ lại: không tạo trùng
	r = e.do(t, http.MethodPost, "/api/v1/admin/inbox/sync", admin.AccessToken, nil)
	decodeData(t, r, &st)
	if st.Added != 0 || st.Total != 4 {
		t.Fatalf("resync: %s", r.Raw)
	}

	ctx := context.Background()
	type row struct {
		id     int64
		ctv    int64
		status string
		dupOf  *int64
		phone  string
		lat    *float64
	}
	get := func(ext string) row {
		var x row
		if err := e.db.QueryRow(ctx, `SELECT id, ctv_id, status, dup_of, contractor_phone, lat FROM houses WHERE ext_id = $1`, ext).
			Scan(&x.id, &x.ctv, &x.status, &x.dupOf, &x.phone, &x.lat); err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		return x
	}
	h1, h2, h3, h4 := get("log-1"), get("log-2"), get("log-3"), get("log-4")
	if h1.status != model.StatusPending || h1.phone != "0943488882" || h1.lat == nil || *h1.lat < 11.07 || *h1.lat > 11.071 {
		t.Fatalf("h1: %+v", h1)
	}
	if h2.status != model.StatusRejected || h2.dupOf == nil || *h2.dupOf != h1.id {
		t.Fatalf("h2 phải tự từ chối vì trùng h1: %+v", h2)
	}
	if h3.status != model.StatusPending || h3.ctv != h1.ctv {
		t.Fatalf("h3 cùng CTV A: %+v", h3)
	}
	if h4.ctv != appCTV.User.ID {
		t.Fatalf("h4 phải ghép vào CTV app cùng SĐT: %+v", h4)
	}

	// CTV tự tạo: A theo SĐT (đổi +84 -> 0), có số tài khoản; B không có SĐT; chưa đăng nhập được
	var loginA, bankA, loginB string
	_ = e.db.QueryRow(ctx, `SELECT username, bank FROM users WHERE id = $1`, h1.ctv).Scan(&loginA, &bankA)
	_ = e.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, h2.ctv).Scan(&loginB)
	if loginA != "0901111111" || !strings.Contains(bankA, "Vietcombank") || !strings.Contains(bankA, "0123456789") ||
		!strings.HasPrefix(loginB, "mess_") {
		t.Fatalf("users: %q %q %q", loginA, bankA, loginB)
	}
	r = e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": loginA, "password": ""})
	if r.Status == http.StatusOK {
		t.Fatalf("CTV tạo từ Messenger không được đăng nhập khi chưa có mật khẩu")
	}

	// Chi tiết h2: lý do + lịch sử do Hệ thống ghi
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", h2.id), admin.AccessToken, nil)
	var det service.HouseDetail
	decodeData(t, r, &det)
	if det.House.Source != "inbox" || !strings.Contains(det.House.Note, fmt.Sprintf("#%d", h1.id)) || len(det.Photos) != 5 {
		t.Fatalf("h2 detail: %s", r.Raw)
	}
	sys := false
	for _, l := range det.Logs {
		if l.Action == service.ActAutoDup && l.UserName == "Hệ thống" {
			sys = true
		}
	}
	if !sys {
		t.Fatalf("thiếu lịch sử tự từ chối: %s", r.Raw)
	}

	// Không qua AI: giờ chụp = giờ CTV gửi tin, nên 6 ảnh đều đạt và có tiền; không có trạng thái AI; chụp đầu tiên: VIP
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", h1.id), admin.AccessToken, nil)
	decodeData(t, r, &det)
	if det.Valid != 6 || det.House.Amount == 0 || det.House.AIState != "" || det.Photos[0].ExifTime == "" || !det.House.VIP {
		t.Fatalf("h1 (Messenger, không AI): valid=%d amount=%d ai=%q exif=%q vip=%v", det.Valid, det.House.Amount,
			det.House.AIState, det.Photos[0].ExifTime, det.House.VIP)
	}

	// Bản đồ: 4 điểm, bảng CTV (A: 2 công trình, B: 0 + 1 trùng)
	r = e.do(t, http.MethodGet, "/api/v1/map", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var m struct {
		Points []repository.MapPoint `json:"points"`
		CTVs   []repository.CTVSites `json:"ctvs"`
	}
	decodeData(t, r, &m)
	if len(m.Points) != 4 {
		t.Fatalf("map points: %s", r.Raw)
	}
	for _, p := range m.Points {
		if p.LastAt.IsZero() || (p.ID == h1.id && !p.VIP) {
			t.Fatalf("map point thiếu last_at / VIP: %+v", p)
		}
	}
	counts := map[int64]repository.CTVSites{}
	for _, c := range m.CTVs {
		counts[c.ID] = c
	}
	if counts[h1.ctv].Total != 2 || counts[h1.ctv].Inbox != 2 || counts[h2.ctv].Total != 0 || counts[h2.ctv].Dups != 1 {
		t.Fatalf("ctv counts: %+v", m.CTVs)
	}
	// lọc theo CTV
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/map?ctv=%d", h1.ctv), admin.AccessToken, nil)
	decodeData(t, r, &m)
	if len(m.Points) != 2 {
		t.Fatalf("map ctv filter: %d", len(m.Points))
	}
	// 34 tỉnh mới: toạ độ Bình Dương cũ -> TP. Hồ Chí Minh (VN79), Vinh -> Nghệ An, Huế -> Huế
	prov := func(id int64) string {
		var p string
		_ = e.db.QueryRow(ctx, `SELECT COALESCE(province, '') FROM houses WHERE id = $1`, id).Scan(&p)
		return p
	}
	if prov(h1.id) != "VN79" || prov(h2.id) != "VN79" || prov(h3.id) != "VN40" || prov(h4.id) != "VN46" {
		t.Fatalf("tỉnh: %s %s %s %s", prov(h1.id), prov(h2.id), prov(h3.id), prov(h4.id))
	}
	var pm struct {
		Points    []repository.MapPoint   `json:"points"`
		Provinces []service.ProvinceSites `json:"provinces"`
	}
	r = e.do(t, http.MethodGet, "/api/v1/map?province=VN79", admin.AccessToken, nil)
	decodeData(t, r, &pm)
	if len(pm.Provinces) != 34 || len(pm.Points) != 2 { // h1 + h2 (bị từ chối vẫn hiện trên bản đồ)
		t.Fatalf("map tỉnh: %d tỉnh, %d điểm", len(pm.Provinces), len(pm.Points))
	}
	for _, p := range pm.Provinces {
		if p.Code == "VN79" && (p.Total != 1 || p.Name != "TP. Hồ Chí Minh" || len(p.Merged) != 3) {
			t.Fatalf("VN79: %+v", p)
		}
	}
	r = e.do(t, http.MethodGet, "/api/v1/houses?province=VN40", admin.AccessToken, nil)
	var ov struct {
		Found int           `json:"found"`
		Rows  []model.House `json:"rows"`
	}
	decodeData(t, r, &ov)
	if ov.Found != 1 || ov.Rows[0].ProvinceName != "Nghệ An" {
		t.Fatalf("lọc hồ sơ theo tỉnh: %s", r.Raw)
	}
	if g := e.getRaw(t, "/api/v1/geo/provinces", ""); g.Status != 200 || !strings.Contains(string(g.Raw), "VN79") {
		t.Fatalf("geojson: %d", g.Status)
	}

	// CTV không xem được bản đồ
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/map", appCTV.AccessToken, nil), http.StatusForbidden)
}

// App: công trình đã có hồ sơ trong 30 ngày (cùng chỗ + cùng nhà thầu) thì không gửi duyệt được; quá hạn thì được.
func TestSiteDuplicateBlocked(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv1 := e.register(t, "CTV một")
	ctv2 := e.register(t, "CTV hai")

	h1 := e.newHouseP(t, ctv1, 5, 7000, 10.90, "0901 234 567")
	// CTV khác, cách ~11m, cùng nhà thầu: bị chặn
	h2, r := e.uploadAndSave(t, ctv2, 5, 7100, 10.9001, "0901234567")
	var saved struct {
		Message   string `json:"message"`
		Submitted bool   `json:"submitted"`
	}
	decodeData(t, r, &saved)
	if saved.Submitted || !strings.Contains(saved.Message, "đã có người chụp") || !strings.Contains(saved.Message, fmt.Sprintf("#%d", h1)) {
		t.Fatalf("phải chặn: %s", r.Raw)
	}
	// chính CTV đó nộp lại cũng bị chặn
	_, r = e.uploadAndSave(t, ctv1, 5, 7200, 10.90002, "0901234567")
	decodeData(t, r, &saved)
	if saved.Submitted {
		t.Fatalf("CTV nộp lại công trình của mình cũng phải chặn: %s", r.Raw)
	}
	// hồ sơ 1 gửi đã 31 ngày: hết hạn, hồ sơ 2 gửi lại được
	if _, err := e.db.Exec(context.Background(), `UPDATE houses SET submitted_at = NOW() - INTERVAL '31 days' WHERE id = $1`, h1); err != nil {
		t.Fatal(err)
	}
	r = e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/houses/%d", h2), ctv2.AccessToken, map[string]any{
		"address": "12 Lê Lợi", "floors": 3, "stage": model.Stages[1], "contractor_name": "Xây dựng ABC",
		"contractor_phone": "0901234567", "submit": true,
	})
	decodeData(t, r, &saved)
	if !saved.Submitted {
		t.Fatalf("quá hạn phải gửi được: %s", r.Raw)
	}
	// admin đổi bán kính / số ngày
	admin := e.staff(t, model.RoleAdmin)
	r = e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken, map[string]any{"price_min": 35000, "price_more": 50000,
		"min_photos": 5, "pay_min": 5, "dup_days": 45, "dup_radius_m": 5})
	expectStatus(t, r, http.StatusBadRequest) // dưới 10m
	r = e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken, map[string]any{"price_min": 35000, "price_more": 50000,
		"min_photos": 5, "pay_min": 5, "dup_days": 45, "dup_radius_m": 80})
	expectStatus(t, r, http.StatusOK)
	r = e.do(t, http.MethodGet, "/api/v1/settings", admin.AccessToken, nil)
	var s model.Settings
	decodeData(t, r, &s)
	if s.DupDays != 45 || s.DupRadiusM != 80 {
		t.Fatalf("settings: %s", r.Raw)
	}
}
