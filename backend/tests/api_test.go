package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"vuathau/backend/internal/config"
	"vuathau/backend/internal/handler"
	"vuathau/backend/internal/integration/google"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/token"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/router"
	"vuathau/backend/internal/service"
	"vuathau/backend/internal/storage"
	"vuathau/backend/internal/websocket"
)

const (
	testSecret  = "test-secret-test-secret-test-secret"
	allowedOrig = "http://localhost:5173"
	testPass    = "s3cret-password"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type testEnv struct {
	t     *testing.T
	srv   *httptest.Server
	hub   *websocket.Hub
	db    *pgxpool.Pool
	house service.HouseService
	data  string // DATA_DIR của máy chủ test
	seq   atomic.Int64
}

type envSettings struct {
	cfg          *config.Config
	healthChecks map[string]router.HealthCheck
}

type envOption func(*envSettings)

func withRateLimit(rps float64, burst int) envOption {
	return func(s *envSettings) {
		s.cfg.RateLimitRPS = rps
		s.cfg.RateLimitBurst = burst
	}
}

func withHealthChecks(checks map[string]router.HealthCheck) envOption {
	return func(s *envSettings) { s.healthChecks = checks }
}

func withCORSOrigins(origins ...string) envOption {
	return func(s *envSettings) { s.cfg.CORSAllowedOrigins = origins }
}

// newTestEnv dựng service, handler, hub, router thật trên 1 CSDL Postgres riêng và chạy bằng httptest.
// AI, Google Drive, tra địa chỉ: tắt (không gọi dịch vụ ngoài khi test).
func newTestEnv(t *testing.T, opts ...envOption) *testEnv {
	t.Helper()

	cfg := &config.Config{
		AppEnv:             "test",
		JWTSecret:          testSecret,
		JWTExpiry:          time.Hour,
		CORSAllowedOrigins: []string{allowedOrig},
		RateLimitRPS:       1000,
		RateLimitBurst:     1000,
		DataDir:            t.TempDir(),
	}
	settings := &envSettings{
		cfg: cfg,
		healthChecks: map[string]router.HealthCheck{
			"postgres": func(context.Context) error { return nil },
			"redis":    func(context.Context) error { return nil },
		},
	}
	for _, o := range opts {
		o(settings)
	}

	db := newDB(t)
	files, err := storage.New(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	hub := websocket.NewHub(cfg.CORSAllowedOrigins)

	authRepo := repository.NewAuthRepository(db)
	userRepo := repository.NewUserRepository(db)
	houseRepo := repository.NewHouseRepository(db)
	photoRepo := repository.NewPhotoRepository(db)
	logRepo := repository.NewLogRepository(db)
	payRepo := repository.NewPaymentRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)

	authSvc := service.NewAuthService(authRepo, noGoogle{}, cfg.JWTSecret, cfg.JWTExpiry)
	userSvc := service.NewUserService(userRepo, authRepo, houseRepo, photoRepo, payRepo, logRepo, settingsRepo)
	msgSvc := service.NewMessageService(repository.NewMessageRepository(db), hub)
	houseSvc := service.NewHouseService(service.HouseDeps{Houses: houseRepo, Photos: photoRepo, Logs: logRepo, Users: userRepo,
		Payments: payRepo, Settings: settingsRepo, Files: files, Public: "http://test"})
	paySvc := service.NewPaymentService(payRepo, houseRepo, photoRepo, userRepo, logRepo, settingsRepo, files)

	deps := router.Deps{
		Config:         cfg,
		AuthHandler:    handler.NewAuthHandler(authSvc),
		UserHandler:    handler.NewUserHandler(userSvc, authSvc),
		MessageHandler: handler.NewMessageHandler(msgSvc),
		HouseHandler:   handler.NewHouseHandler(houseSvc),
		PaymentHandler: handler.NewPaymentHandler(paySvc, ""),
		UserLookup:     userRepo.GetByID,
		Hub:            hub,
		HealthChecks:   settings.healthChecks,
	}

	srv := httptest.NewServer(router.New(deps))
	t.Cleanup(func() {
		hub.Close()
		srv.Close()
		houseSvc.Wait()
	})
	return &testEnv{t: t, srv: srv, hub: hub, db: db, house: houseSvc, data: cfg.DataDir}
}

type noGoogle struct{}

func (noGoogle) Verify(context.Context, string) *google.Identity { return nil }

func mustSignToken(t *testing.T, secret string, userID int64) string {
	t.Helper()
	tok, err := token.Generate(secret, userID, 0, time.Hour)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

type apiResp struct {
	Status int
	Header http.Header
	Raw    []byte
	Body   envelope
}

// do sends a request with an optional JSON body and bearer token. A string
// body is sent verbatim (used for malformed JSON).
func (e *testEnv) do(t *testing.T, method, path, tok string, body any) apiResp {
	t.Helper()

	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rdr = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, e.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return e.send(t, req)
}

func (e *testEnv) send(t *testing.T, req *http.Request) apiResp {
	t.Helper()
	out := e.sendRaw(t, req)
	if len(bytes.TrimSpace(out.Raw)) > 0 {
		if err := json.Unmarshal(out.Raw, &out.Body); err != nil {
			t.Fatalf("%s %s: response is not a JSON envelope: %v\n%s", req.Method, req.URL.Path, err, out.Raw)
		}
	}
	return out
}

// sendRaw: không đòi JSON (ảnh, Excel).
func (e *testEnv) sendRaw(t *testing.T, req *http.Request) apiResp {
	t.Helper()
	res, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return apiResp{Status: res.StatusCode, Header: res.Header, Raw: raw}
}

func (e *testEnv) getRaw(t *testing.T, path, tok string) apiResp {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.sendRaw(t, req)
}

// upload gửi multipart: các file ở trường field + các trường chữ.
func (e *testEnv) upload(t *testing.T, path, tok, field string, files [][]byte, fields map[string]string) apiResp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for i, f := range files {
		fw, err := w.CreateFormFile(field, fmt.Sprintf("anh-%d.jpg", i+1))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(f)
	}
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.send(t, req)
}

func expectStatus(t *testing.T, r apiResp, want int) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("status = %d, want %d; body: %s", r.Status, want, r.Raw)
	}
}

func decodeData(t *testing.T, r apiResp, dst any) {
	t.Helper()
	if err := json.Unmarshal(r.Body.Data, dst); err != nil {
		t.Fatalf("decode data: %v\n%s", err, r.Raw)
	}
}

func assertNoPasswordLeak(t *testing.T, r apiResp) {
	t.Helper()
	if bytes.Contains(bytes.ToLower(r.Raw), []byte("password")) {
		t.Fatalf("response leaks a password field: %s", r.Raw)
	}
}

type authData struct {
	User        model.User `json:"user"`
	AccessToken string     `json:"access_token"`
	ExpiresIn   int64      `json:"expires_in"`
}

// newPhone: SĐT duy nhất trong env.
func (e *testEnv) newPhone() string { return fmt.Sprintf("09%08d", e.seq.Add(1)) }

func ctvBody(name, phone string) map[string]string {
	return map[string]string{"name": name, "phone": phone, "password": testPass,
		"bank_name": "Vietcombank", "bank_no": "0123456789", "bank_owner": "nguyen van a"}
}

// register: CTV đăng ký bằng SĐT.
func (e *testEnv) register(t *testing.T, name string) authData {
	t.Helper()
	r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", ctvBody(name, e.newPhone()))
	expectStatus(t, r, http.StatusCreated)
	var a authData
	decodeData(t, r, &a)
	if a.AccessToken == "" || a.User.ID == 0 {
		t.Fatalf("register returned incomplete data: %s", r.Raw)
	}
	return a
}

// staff tạo tài khoản admin / kế toán thẳng trong CSDL rồi đăng nhập.
func (e *testEnv) staff(t *testing.T, role string) authData {
	t.Helper()
	login := fmt.Sprintf("%s_%d", role, e.seq.Add(1))
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPass), bcrypt.MinCost)
	if _, err := e.db.Exec(context.Background(), `INSERT INTO users (username, name, role, password_hash) VALUES ($1, $2, $3, $4)`,
		login, "NV "+role, role, string(hash)); err != nil {
		t.Fatalf("tạo nhân viên: %v", err)
	}
	return e.login(t, login, testPass)
}

func (e *testEnv) login(t *testing.T, login, pw string) authData {
	t.Helper()
	r := e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": login, "password": pw})
	expectStatus(t, r, http.StatusOK)
	var a authData
	decodeData(t, r, &a)
	return a
}

// photo: JPEG ngẫu nhiên khác nhau (dHash khác nhau) để không bị coi là ảnh trùng.
func photo(t *testing.T, seed uint64) []byte {
	t.Helper()
	rnd := rand.New(rand.NewPCG(seed, seed*7+1))
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for by := 0; by < 8; by++ {
		for bx := 0; bx < 9; bx++ {
			c := color.RGBA{uint8(rnd.IntN(256)), uint8(rnd.IntN(256)), uint8(rnd.IntN(256)), 255}
			for y := by * 15; y < (by+1)*15; y++ {
				for x := bx * 18; x < min(160, (bx+1)*18); x++ {
					img.Set(x, y, c)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// cameraMeta: thông tin camera app gửi kèm (giờ chụp, GPS) cho n ảnh.
func cameraMeta(n int, lat float64) string {
	t := time.Now().In(service.VN).Format("2006-01-02 15:04")
	ms := make([]map[string]any, n)
	for i := range ms {
		ms[i] = map[string]any{"t": t, "lat": lat, "lng": 106.7, "acc": 8, "addr": "12 Lê Lợi, Quận 1, TP HCM"}
	}
	b, _ := json.Marshal(ms)
	return string(b)
}

// ---------------------------------------------------------------------------
// 1. Health
// ---------------------------------------------------------------------------

type healthData struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

func TestHealth(t *testing.T) {
	t.Parallel()

	t.Run("all up", func(t *testing.T) {
		t.Parallel()
		e := newTestEnv(t)
		r := e.do(t, http.MethodGet, "/health", "", nil)
		expectStatus(t, r, http.StatusOK)
		var h healthData
		decodeData(t, r, &h)
		if h.Status != "ok" || h.Services["postgres"] != "up" || h.Services["redis"] != "up" {
			t.Fatalf("unexpected health: %+v", h)
		}
	})

	t.Run("dependency down", func(t *testing.T) {
		t.Parallel()
		e := newTestEnv(t, withHealthChecks(map[string]router.HealthCheck{
			"postgres": func(context.Context) error { return nil },
			"redis":    func(context.Context) error { return errors.New("connection refused") },
		}))
		r := e.do(t, http.MethodGet, "/health", "", nil)
		expectStatus(t, r, http.StatusServiceUnavailable)
		var h healthData
		decodeData(t, r, &h)
		if h.Status != "degraded" || h.Services["redis"] != "down" || r.Body.Success {
			t.Fatalf("unexpected health: %+v (success=%v)", h, r.Body.Success)
		}
	})

	t.Run("unknown api route", func(t *testing.T) {
		t.Parallel()
		e := newTestEnv(t)
		r := e.do(t, http.MethodGet, "/api/v1/nope", "", nil)
		expectStatus(t, r, http.StatusNotFound)
		if r.Body.Success || r.Body.Error == "" {
			t.Fatalf("404 body is not an error envelope: %s", r.Raw)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Auth
// ---------------------------------------------------------------------------

func TestAuthFlow(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)

	phone := e.newPhone()
	r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", ctvBody("Nguyễn Văn A", phone))
	expectStatus(t, r, http.StatusCreated)
	assertNoPasswordLeak(t, r)
	var reg authData
	decodeData(t, r, &reg)
	if reg.User.Role != model.RoleCTV || reg.User.Username != phone || reg.ExpiresIn != 3600 {
		t.Fatalf("unexpected register data: %+v", reg)
	}
	if reg.User.Bank != "Vietcombank · 0123456789 · NGUYEN VAN A" {
		t.Fatalf("bank = %q", reg.User.Bank)
	}

	// SĐT gõ có dấu cách vẫn đăng nhập được
	login := e.login(t, phone[:4]+" "+phone[4:], testPass)
	for _, tok := range []string{reg.AccessToken, login.AccessToken} {
		r = e.do(t, http.MethodGet, "/api/v1/me", tok, nil)
		expectStatus(t, r, http.StatusOK)
		assertNoPasswordLeak(t, r)
		var me service.MeResult
		decodeData(t, r, &me)
		if me.User.ID != reg.User.ID || me.User.Name != "Nguyễn Văn A" || me.Bank.No != "0123456789" || me.Money == nil {
			t.Fatalf("unexpected /me: %s", r.Raw)
		}
	}
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/users/999999", login.AccessToken, nil), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/users/abc", login.AccessToken, nil), http.StatusBadRequest)
}

func TestUserPrivacy(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "Alice")
	b := e.register(t, "Bob")

	r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/users/%d", b.User.ID), a.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var u model.User
	decodeData(t, r, &u)
	if u.Bank != "" {
		t.Fatalf("other user's bank leaked: %s", r.Raw)
	}
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/users/%d", a.User.ID), a.AccessToken, nil)
	decodeData(t, r, &u)
	if u.Bank == "" {
		t.Fatalf("own bank should be visible: %s", r.Raw)
	}
}

func TestRegisterValidation(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)

	base := ctvBody("Trần B", "0912345678")
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for kk, vv := range base {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	cases := []struct {
		name string
		body any
	}{
		{"malformed json", `{"name":`},
		{"bad phone", with("phone", "12345")},
		{"short password", with("password", "123")},
		{"no name", with("name", "  ")},
		{"no bank", with("bank_name", "")},
		{"bank number not digits", with("bank_no", "abc123")},
		{"no bank owner", with("bank_owner", "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", tc.body)
			expectStatus(t, r, http.StatusBadRequest)
			if r.Body.Success || r.Body.Error == "" {
				t.Fatalf("expected error envelope: %s", r.Raw)
			}
		})
	}
}

func TestRegisterDuplicate(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	phone := e.newPhone()
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/auth/register", "", ctvBody("C", phone)), http.StatusCreated)
	r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", ctvBody("C2", phone))
	expectStatus(t, r, http.StatusConflict)
}

func TestLoginInvalidCredentials(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "Dave")

	for _, body := range []map[string]string{
		{"login": a.User.Username, "password": "wrong-password"},
		{"login": "0999999999", "password": testPass},
	} {
		r := e.do(t, http.MethodPost, "/api/v1/auth/login", "", body)
		expectStatus(t, r, http.StatusUnauthorized)
	}

	// sai 5 lần liền: khoá đăng nhập 5 phút, kể cả đúng mật khẩu
	for range 5 {
		e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": a.User.Username, "password": "x"})
	}
	r := e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": a.User.Username, "password": testPass})
	expectStatus(t, r, http.StatusTooManyRequests)
}

func TestProtectedRoutesRequireToken(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/me"},
		{http.MethodGet, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users/1"},
		{http.MethodPost, "/api/v1/messages"},
		{http.MethodGet, "/api/v1/messages/1"},
		{http.MethodGet, "/api/v1/ctv/home"},
		{http.MethodPost, "/api/v1/houses"},
		{http.MethodGet, "/api/v1/houses/1"},
		{http.MethodGet, "/api/v1/photos/1"},
		{http.MethodGet, "/api/v1/admin/queue"},
		{http.MethodGet, "/api/v1/accounting"},
		{http.MethodGet, "/api/v1/reports"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			t.Parallel()
			expectStatus(t, e.do(t, rt.method, rt.path, "", nil), http.StatusUnauthorized)
			expectStatus(t, e.do(t, rt.method, rt.path, "not-a-jwt", nil), http.StatusUnauthorized)
			expired, _ := token.Generate(testSecret, 1, 0, -time.Minute)
			expectStatus(t, e.do(t, rt.method, rt.path, expired, nil), http.StatusUnauthorized)
			wrongKey := mustSignToken(t, "another-secret-another-secret-xx", 1)
			expectStatus(t, e.do(t, rt.method, rt.path, wrongKey, nil), http.StatusUnauthorized)
		})
	}
}

func TestRoles(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV")
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)

	cases := []struct {
		who  authData
		path string
		want int
	}{
		{ctv, "/api/v1/admin/queue", http.StatusForbidden},
		{ctv, "/api/v1/accounting", http.StatusForbidden},
		{ctv, "/api/v1/houses", http.StatusForbidden},
		{admin, "/api/v1/admin/queue", http.StatusOK},
		{admin, "/api/v1/accounting", http.StatusForbidden},
		{admin, "/api/v1/ctv/home", http.StatusForbidden},
		{kt, "/api/v1/accounting", http.StatusOK},
		{kt, "/api/v1/admin/queue", http.StatusForbidden},
		{kt, "/api/v1/houses", http.StatusOK},
		{kt, "/api/v1/settings", http.StatusForbidden},
		{ctv, "/api/v1/ctv/home", http.StatusOK},
	}
	for _, c := range cases {
		expectStatus(t, e.do(t, http.MethodGet, c.path, c.who.AccessToken, nil), c.want)
	}
}

func TestPasswordChangeAndLock(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "Eve")
	admin := e.staff(t, model.RoleAdmin)

	r := e.do(t, http.MethodPut, "/api/v1/me/password", a.AccessToken, map[string]string{"old": testPass, "new": "new-password-1"})
	expectStatus(t, r, http.StatusOK)
	var res struct {
		AccessToken string `json:"access_token"`
	}
	decodeData(t, r, &res)
	// token cũ hết hiệu lực, token mới dùng được
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", a.AccessToken, nil), http.StatusUnauthorized)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", res.AccessToken, nil), http.StatusOK)
	e.login(t, a.User.Username, "new-password-1")

	// admin khoá: token hết hiệu lực, không đăng nhập được
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/lock", a.User.ID), admin.AccessToken,
		map[string]bool{"locked": true}), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", res.AccessToken, nil), http.StatusUnauthorized)
	r = e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": a.User.Username, "password": "new-password-1"})
	expectStatus(t, r, http.StatusForbidden)
}

// ---------------------------------------------------------------------------
// 3. Nghiệm thu: CTV nộp -> admin duyệt -> kế toán trả tiền
// ---------------------------------------------------------------------------

type uploadRes struct {
	Message  string   `json:"message"`
	Messages []string `json:"messages"`
	HouseID  *int64   `json:"house_id"`
	Added    int      `json:"added"`
}

// newHouse: CTV nộp 1 hồ sơ n ảnh (camera app, có giờ + GPS), điền thông tin và gửi duyệt.
func (e *testEnv) newHouse(t *testing.T, ctv authData, n int, seed uint64, lat float64) int64 {
	t.Helper()
	files := make([][]byte, n)
	for i := range files {
		files[i] = photo(t, seed+uint64(i))
	}
	r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(n, lat)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	if up.HouseID == nil || up.Added != n {
		t.Fatalf("upload: %s", r.Raw)
	}
	r = e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/houses/%d", *up.HouseID), ctv.AccessToken, map[string]any{
		"address": "12 Lê Lợi", "floors": 3, "stage": model.Stages[1], "contractor_name": "Xây dựng ABC",
		"contractor_phone": "0901 234 567", "submit": true,
	})
	expectStatus(t, r, http.StatusOK)
	var saved struct {
		Message   string `json:"message"`
		Submitted bool   `json:"submitted"`
	}
	decodeData(t, r, &saved)
	if !saved.Submitted {
		t.Fatalf("not submitted: %s", r.Raw)
	}
	return *up.HouseID
}

func TestNghiemThuFlow(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "Nguyễn Văn CTV")
	admin := e.staff(t, model.RoleAdmin)
	kt := e.staff(t, model.RoleKetoan)

	// Cấu hình mặc định qua /config
	r := e.do(t, http.MethodGet, "/api/v1/config", "", nil)
	expectStatus(t, r, http.StatusOK)
	var cfg service.Config
	decodeData(t, r, &cfg)
	if cfg.MinPhotos != 5 || cfg.Pay[5] != 35000 || cfg.Pay[6] != 50000 || len(cfg.Stages) != 4 {
		t.Fatalf("config: %s", r.Raw)
	}

	// Thiếu ảnh: không gửi duyệt được
	files := [][]byte{photo(t, 100), photo(t, 101)}
	r = e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(2, 10.80)})
	expectStatus(t, r, http.StatusCreated)
	var up uploadRes
	decodeData(t, r, &up)
	r = e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/houses/%d", *up.HouseID), ctv.AccessToken, map[string]any{"submit": true})
	expectStatus(t, r, http.StatusOK)
	if !strings.Contains(string(r.Raw), "Chưa gửi được") {
		t.Fatalf("expected missing items: %s", r.Raw)
	}

	// Ảnh trùng hệt: bị từ chối, không tạo hồ sơ
	r = e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", [][]byte{files[0]}, nil)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &up)
	if up.HouseID != nil || len(up.Messages) != 1 || !strings.Contains(up.Messages[0], "đã nộp") {
		t.Fatalf("duplicate upload: %s", r.Raw)
	}

	// Kiểm 1 ảnh trước khi nộp: ảnh không có giờ / vị trí
	r = e.upload(t, "/api/v1/photos/check", ctv.AccessToken, "file", [][]byte{photo(t, 900)}, nil)
	expectStatus(t, r, http.StatusOK)
	var chk service.CheckResult
	decodeData(t, r, &chk)
	if len(chk.Problems) != 2 {
		t.Fatalf("check problems = %v", chk.Problems)
	}

	// Hồ sơ đủ 6 ảnh
	hid := e.newHouse(t, ctv, 6, 200, 10.80)

	// Chi tiết: CTV và admin xem được, ảnh tải được, CTV khác không xem được
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var det service.HouseDetail
	decodeData(t, r, &det)
	if det.House.Status != model.StatusPending || det.Valid != 6 || det.House.Amount != 50000 || len(det.Steps) != 4 || det.Editable {
		t.Fatalf("detail: %s", r.Raw)
	}
	if ph := e.getRaw(t, fmt.Sprintf("/api/v1/photos/%d", det.Photos[0].ID), ctv.AccessToken); ph.Status != 200 ||
		ph.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("photo status %d %s", ph.Status, ph.Header.Get("Content-Type"))
	}
	other := e.register(t, "Người khác")
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), other.AccessToken, nil), http.StatusNotFound)
	// đang chờ duyệt: CTV không sửa được nữa
	expectStatus(t, e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/houses/%d", hid), ctv.AccessToken, map[string]any{}), http.StatusForbidden)

	// Nhà thứ 2 sát nhà 1 (< 30m): cảnh báo trùng nhà
	hid2 := e.newHouse(t, ctv, 5, 300, 10.80005)
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid2), admin.AccessToken, nil)
	decodeData(t, r, &det)
	if !strings.Contains(det.House.Dup, fmt.Sprintf("#%d", hid)) || det.House.Amount != 35000 {
		t.Fatalf("dup warning: %q amount %d", det.House.Dup, det.House.Amount)
	}

	// Admin: hàng chờ, trả lại cần lý do, trả lại, duyệt
	r = e.do(t, http.MethodGet, "/api/v1/admin/queue", admin.AccessToken, nil)
	var q struct {
		Rows []model.House `json:"rows"`
	}
	decodeData(t, r, &q)
	if len(q.Rows) != 2 || q.Rows[0].ID != hid || q.Rows[0].CoverID == nil || q.Rows[0].PhotosValid != 6 {
		t.Fatalf("queue: %s", r.Raw)
	}
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/houses/%d/review", hid2), admin.AccessToken,
		map[string]string{"action": "tra_lai"}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/houses/%d/review", hid2), admin.AccessToken,
		map[string]string{"action": "tra_lai", "reason": "Ảnh mờ"}), http.StatusOK)
	r = e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/houses/%d/review", hid), admin.AccessToken, map[string]string{"action": "duyet"})
	expectStatus(t, r, http.StatusOK)
	var rv service.ReviewResult
	decodeData(t, r, &rv)
	if rv.Left != 0 || rv.NextID != nil {
		t.Fatalf("review: %s", r.Raw)
	}

	// CTV: hồ sơ bị trả lại hiện trong "Cần xử lý", có lý do
	r = e.do(t, http.MethodGet, "/api/v1/ctv/home?tab=can_lam", ctv.AccessToken, nil)
	var home service.CTVHome
	decodeData(t, r, &home)
	if home.Todo != 2 || len(home.Rows) != 2 || home.Money.Approved != 1 || home.Money.Owed != 50000 {
		t.Fatalf("ctv home: %s", r.Raw)
	}

	// Kế toán: chưa đủ 5 hồ sơ -> chưa trả được. Admin hạ ngưỡng xuống 1.
	r = e.do(t, http.MethodGet, "/api/v1/accounting", kt.AccessToken, nil)
	var acc service.Accounting
	decodeData(t, r, &acc)
	if len(acc.Ready) != 0 || len(acc.Waiting) != 1 {
		t.Fatalf("accounting before: %s", r.Raw)
	}
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken,
		model.Settings{PriceMin: 35000, PriceMore: 50000, MinPhotos: 5, PayMin: 1}), http.StatusOK)

	// Thưởng 10.000đ, rồi trả
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/ctvs/%d/adjustments", ctv.User.ID), kt.AccessToken,
		map[string]any{"amount": 10000, "reason": "Thưởng tháng"}), http.StatusCreated)
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/payouts/%d", ctv.User.ID), kt.AccessToken, nil)
	var po service.PayoutPage
	decodeData(t, r, &po)
	if po.Total != 60000 || len(po.Rows) != 1 || len(po.Adjustments) != 1 || po.Bank.No != "0123456789" {
		t.Fatalf("payout page: %s", r.Raw)
	}
	// Danh sách đã cũ (thiếu khoản thưởng): từ chối, tránh trả nhầm
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/payouts/%d", ctv.User.ID), kt.AccessToken,
		map[string]any{"house_ids": []int64{hid}, "adjustment_ids": []int64{}}), http.StatusConflict)
	r = e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/payouts/%d", ctv.User.ID), kt.AccessToken,
		map[string]any{"house_ids": []int64{hid}, "adjustment_ids": []int64{po.Adjustments[0].ID}, "ref": "FT123"})
	expectStatus(t, r, http.StatusCreated)
	var paid struct {
		BatchID int64 `json:"batch_id"`
	}
	decodeData(t, r, &paid)

	// Trả lần 2 cùng dữ liệu: không còn gì để trả
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/payouts/%d", ctv.User.ID), kt.AccessToken,
		map[string]any{"house_ids": []int64{hid}, "adjustment_ids": []int64{po.Adjustments[0].ID}}), http.StatusConflict)

	// CTV thấy tiền đã nhận, xem được phiếu chi của mình
	r = e.do(t, http.MethodGet, "/api/v1/ctv/income", ctv.AccessToken, nil)
	var inc service.Income
	decodeData(t, r, &inc)
	if inc.Money.Paid != 60000 || inc.Money.Owed != 0 || len(inc.Pays) != 1 || inc.Pays[0].Total != 60000 {
		t.Fatalf("income: %s", r.Raw)
	}
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/batches/%d", paid.BatchID), ctv.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var bd service.BatchDetail
	decodeData(t, r, &bd)
	if bd.Total != 60000 || bd.Batch.Ref != "FT123" || len(bd.Logs) != 0 {
		t.Fatalf("batch for ctv: %s", r.Raw)
	}
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/batches/%d", paid.BatchID), other.AccessToken, nil), http.StatusNotFound)

	// Biên lai: kế toán tải lên, CTV xem được
	r = e.upload(t, fmt.Sprintf("/api/v1/batches/%d/receipt", paid.BatchID), kt.AccessToken, "file", [][]byte{photo(t, 777)}, nil)
	expectStatus(t, r, http.StatusOK)
	if rc := e.getRaw(t, fmt.Sprintf("/api/v1/batches/%d/receipt", paid.BatchID), ctv.AccessToken); rc.Status != 200 {
		t.Fatalf("receipt status %d", rc.Status)
	}

	// Báo cáo + Excel
	r = e.do(t, http.MethodGet, "/api/v1/reports?group=month", admin.AccessToken, nil)
	var rep service.Report
	decodeData(t, r, &rep)
	if rep.Total != 60000 || rep.Batches != 1 || rep.Houses != 1 || rep.Flow.Approved != 1 || rep.Flow.Returned != 1 {
		t.Fatalf("report: %s", r.Raw)
	}
	if x := e.getRaw(t, "/api/v1/reports/export", admin.AccessToken); x.Status != 200 || !bytes.HasPrefix(x.Raw, []byte("PK")) {
		t.Fatalf("excel export: status %d", x.Status)
	}

	// Huỷ phiếu chi: hồ sơ về chờ thanh toán, CTV hết thấy phiếu
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/batches/%d/cancel", paid.BatchID), kt.AccessToken,
		map[string]string{}), http.StatusBadRequest)
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/batches/%d/cancel", paid.BatchID), kt.AccessToken,
		map[string]string{"reason": "Ghi nhầm"}), http.StatusOK)
	r = e.do(t, http.MethodGet, "/api/v1/ctv/income", ctv.AccessToken, nil)
	decodeData(t, r, &inc)
	if inc.Money.Paid != 0 || inc.Money.Owed != 60000 || len(inc.Pays) != 0 {
		t.Fatalf("income after cancel: %s", r.Raw)
	}
	r = e.do(t, http.MethodGet, "/api/v1/accounting", kt.AccessToken, nil)
	decodeData(t, r, &acc)
	if len(acc.Ready) != 1 || acc.Total != 60000 {
		t.Fatalf("accounting after cancel: %s", r.Raw)
	}
	if x := e.getRaw(t, "/api/v1/accounting/export?format=csv", kt.AccessToken); x.Status != 200 ||
		!strings.Contains(string(x.Raw), "0123456789") {
		t.Fatalf("csv export: %d %s", x.Status, x.Raw)
	}

	// Trả hàng loạt với tổng tiền đang thấy
	r = e.do(t, http.MethodPost, "/api/v1/accounting/pay", kt.AccessToken,
		map[string]any{"ref": "LO1", "items": []map[string]any{{"ctv_id": ctv.User.ID, "total": 60000}}})
	expectStatus(t, r, http.StatusOK)
	var pm service.PayManyResult
	decodeData(t, r, &pm)
	if len(pm.Paid) != 1 || pm.Skipped != 0 {
		t.Fatalf("pay many: %s", r.Raw)
	}

	// Tổng quan + tìm kiếm
	r = e.do(t, http.MethodGet, "/api/v1/houses?q=ABC&status=da_chi", admin.AccessToken, nil)
	var ov service.Overview
	decodeData(t, r, &ov)
	if ov.Found != 1 || ov.Counts[model.StatusPaid] != 1 || ov.Chips[model.StatusReturned] != 1 {
		t.Fatalf("overview: %s", r.Raw)
	}

	// CTV tự xoá tài khoản: không đăng nhập lại được, hồ sơ vẫn còn
	expectStatus(t, e.do(t, http.MethodDelete, "/api/v1/me", ctv.AccessToken, nil), http.StatusOK)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/me", ctv.AccessToken, nil), http.StatusUnauthorized)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": ctv.User.Username, "password": testPass}),
		http.StatusUnauthorized)
	expectStatus(t, e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", hid), admin.AccessToken, nil), http.StatusOK)
}

func TestStaffManagement(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)

	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/staff", admin.AccessToken,
		map[string]string{"login": "ab", "name": "X", "role": "ketoan", "password": "12345678"}), http.StatusBadRequest)
	r := e.do(t, http.MethodPost, "/api/v1/staff", admin.AccessToken,
		map[string]string{"login": "KeToan_Lan", "name": "Lan", "role": "ketoan", "password": "12345678"})
	expectStatus(t, r, http.StatusCreated)
	expectStatus(t, e.do(t, http.MethodPost, "/api/v1/staff", admin.AccessToken,
		map[string]string{"login": "ketoan_lan", "name": "Lan 2", "role": "ketoan", "password": "12345678"}), http.StatusConflict)
	kt := e.login(t, "ketoan_lan", "12345678")
	if kt.User.Role != model.RoleKetoan {
		t.Fatalf("role = %s", kt.User.Role)
	}
	r = e.do(t, http.MethodGet, "/api/v1/staff", admin.AccessToken, nil)
	var st struct {
		Rows []model.User `json:"rows"`
	}
	decodeData(t, r, &st)
	if len(st.Rows) != 2 {
		t.Fatalf("staff: %s", r.Raw)
	}
	// admin không tự khoá mình
	expectStatus(t, e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/lock", admin.User.ID), admin.AccessToken,
		map[string]bool{"locked": true}), http.StatusForbidden)
}

// ---------------------------------------------------------------------------
// 4. Messages (scaffold)
// ---------------------------------------------------------------------------

func TestMessages(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "Alice")
	b := e.register(t, "Bob")
	c := e.register(t, "Carol")

	send := func(tok string, to int64, content string) apiResp {
		return e.do(t, http.MethodPost, "/api/v1/messages", tok, map[string]any{"receiver_id": to, "content": content})
	}
	expectStatus(t, send(a.AccessToken, b.User.ID, "hi bob"), http.StatusCreated)
	expectStatus(t, send(b.AccessToken, a.User.ID, "hi alice"), http.StatusCreated)
	expectStatus(t, send(a.AccessToken, b.User.ID, "how are you"), http.StatusCreated)

	r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d", b.User.ID), a.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var conv []model.Message
	decodeData(t, r, &conv)
	if len(conv) != 3 || conv[0].Content != "how are you" {
		t.Fatalf("conversation: %s", r.Raw)
	}
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d?limit=1&offset=1", b.User.ID), a.AccessToken, nil)
	decodeData(t, r, &conv)
	if len(conv) != 1 || conv[0].Content != "hi alice" {
		t.Fatalf("paged conversation: %s", r.Raw)
	}
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d", c.User.ID), b.AccessToken, nil)
	decodeData(t, r, &conv)
	if len(conv) != 0 {
		t.Fatalf("unrelated conversation not empty: %s", r.Raw)
	}

	t.Run("validation", func(t *testing.T) {
		expectStatus(t, send(a.AccessToken, a.User.ID, "self"), http.StatusBadRequest)
		expectStatus(t, send(a.AccessToken, b.User.ID, "   "), http.StatusBadRequest)
		expectStatus(t, send(a.AccessToken, 999999, "ghost"), http.StatusNotFound)
		expectStatus(t, e.do(t, http.MethodPost, "/api/v1/messages", a.AccessToken, `{"receiver_id":`), http.StatusBadRequest)
		expectStatus(t, e.do(t, http.MethodGet, "/api/v1/messages/abc", a.AccessToken, nil), http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// 5. WebSocket (scaffold)
// ---------------------------------------------------------------------------

func (e *testEnv) wsURL(tok string) string {
	u := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws"
	if tok != "" {
		u += "?token=" + tok
	}
	return u
}

func dialWS(url, origin string) (*gws.Conn, *http.Response, error) {
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	d := gws.Dialer{HandshakeTimeout: 3 * time.Second}
	return d.Dial(url, h)
}

type wsEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func TestWebSocketReceivesNewMessage(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "Alice")
	b := e.register(t, "Bob")

	conn, res, err := dialWS(e.wsURL(b.AccessToken), allowedOrig)
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	defer conn.Close()

	events := make(chan wsEvent, 64)
	go func() {
		defer close(events)
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		for {
			var ev wsEvent
			if err := conn.ReadJSON(&ev); err != nil {
				return
			}
			events <- ev
		}
	}()

	waitFor := func(what string, match func(wsEvent) bool) wsEvent {
		t.Helper()
		for ev := range events {
			if match(ev) {
				return ev
			}
		}
		t.Fatalf("websocket closed or timed out before %s event", what)
		return wsEvent{}
	}

	var synced atomic.Bool
	syncDone := make(chan struct{})
	go func() {
		defer close(syncDone)
		for !synced.Load() {
			e.hub.SendToUser(b.User.ID, map[string]string{"type": "test:sync"})
			time.Sleep(20 * time.Millisecond)
		}
	}()
	waitFor("test:sync", func(ev wsEvent) bool { return ev.Type == "test:sync" })
	synced.Store(true)
	<-syncDone

	r := e.do(t, http.MethodPost, "/api/v1/messages", a.AccessToken, map[string]any{
		"receiver_id": b.User.ID, "content": "real-time hello",
	})
	expectStatus(t, r, http.StatusCreated)

	ev := waitFor("message:new", func(ev wsEvent) bool { return ev.Type == "message:new" })
	var msg model.Message
	if err := json.Unmarshal(ev.Data, &msg); err != nil {
		t.Fatalf("decode event data: %v (%s)", err, ev.Data)
	}
	if msg.Content != "real-time hello" || msg.SenderID != a.User.ID || msg.ReceiverID != b.User.ID || msg.ID == 0 {
		t.Fatalf("unexpected event payload: %+v", msg)
	}
}

func TestWebSocketHandshakeRejected(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	b := e.register(t, "Bob")

	cases := []struct {
		name, tok, origin string
		want              int
	}{
		{"missing token", "", allowedOrig, http.StatusUnauthorized},
		{"garbage token", "garbage", allowedOrig, http.StatusUnauthorized},
		{"disallowed origin", b.AccessToken, "http://evil.example", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conn, res, err := dialWS(e.wsURL(tc.tok), tc.origin)
			if err == nil {
				conn.Close()
				t.Fatal("handshake succeeded, want failure")
			}
			if !errors.Is(err, gws.ErrBadHandshake) {
				t.Fatalf("err = %v, want ErrBadHandshake", err)
			}
			got := 0
			if res != nil {
				got = res.StatusCode
			}
			if got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. CORS
// ---------------------------------------------------------------------------

func TestCORS(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)

	preflight := func(t *testing.T, origin string) apiResp {
		t.Helper()
		req, err := http.NewRequest(http.MethodOptions, e.srv.URL+"/api/v1/auth/login", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "content-type, authorization")
		return e.send(t, req)
	}

	t.Run("allowed origin preflight", func(t *testing.T) {
		t.Parallel()
		r := preflight(t, allowedOrig)
		expectStatus(t, r, http.StatusNoContent)
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != allowedOrig {
			t.Fatalf("Allow-Origin = %q, want %q", got, allowedOrig)
		}
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Fatalf("Allow-Credentials = %q, want true for an explicit origin", got)
		}
		if got := r.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
			t.Fatalf("Allow-Methods = %q, want POST", got)
		}
	})

	t.Run("disallowed origin preflight", func(t *testing.T) {
		t.Parallel()
		r := preflight(t, "http://evil.example")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("Allow-Origin = %q, want none", got)
		}
	})

	t.Run("wildcard origin omits credentials", func(t *testing.T) {
		t.Parallel()
		w := newTestEnv(t, withCORSOrigins("*"))
		req, _ := http.NewRequest(http.MethodOptions, w.srv.URL+"/api/v1/auth/login", nil)
		req.Header.Set("Origin", "http://anything.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		r := w.send(t, req)
		expectStatus(t, r, http.StatusNoContent)
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("Allow-Credentials = %q, want none for wildcard config", got)
		}
	})
}

// ---------------------------------------------------------------------------
// 7. Rate limiting
// ---------------------------------------------------------------------------

func TestRateLimit(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t, withRateLimit(1, 2))

	var statuses []int
	var last apiResp
	for range 3 {
		last = e.do(t, http.MethodGet, "/health", "", nil)
		statuses = append(statuses, last.Status)
	}
	if statuses[0] != http.StatusOK || statuses[1] != http.StatusOK || statuses[2] != http.StatusTooManyRequests {
		t.Fatalf("statuses = %v, want [200 200 429]", statuses)
	}
	if last.Header.Get("Retry-After") == "" {
		t.Fatal("429 response missing Retry-After header")
	}
}
