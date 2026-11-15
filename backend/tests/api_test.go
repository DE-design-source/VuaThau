package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"vuathau/backend/internal/config"
	"vuathau/backend/internal/handler"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/token"
	"vuathau/backend/internal/router"
	"vuathau/backend/internal/service"
	"vuathau/backend/internal/websocket"
)

const (
	testSecret  = "test-secret"
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
	store *memStore
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

// newTestEnv wires the real services, handlers, hub and router on top of the
// in-memory fakes and serves them with httptest.
func newTestEnv(t *testing.T, opts ...envOption) *testEnv {
	t.Helper()

	cfg := &config.Config{
		AppEnv:             "test",
		JWTSecret:          testSecret,
		JWTExpiry:          time.Hour,
		CORSAllowedOrigins: []string{allowedOrig},
		RateLimitRPS:       1000,
		RateLimitBurst:     1000,
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

	store := newMemStore()
	hub := websocket.NewHub(cfg.CORSAllowedOrigins)

	authSvc := service.NewAuthService(&fakeAuthRepo{s: store}, cfg.JWTSecret, cfg.JWTExpiry)
	userSvc := service.NewUserService(&fakeUserRepo{s: store})
	msgSvc := service.NewMessageService(&fakeMessageRepo{s: store}, hub)

	deps := router.Deps{
		Config:         cfg,
		AuthHandler:    handler.NewAuthHandler(authSvc),
		UserHandler:    handler.NewUserHandler(userSvc),
		MessageHandler: handler.NewMessageHandler(msgSvc),
		Hub:            hub,
		HealthChecks:   settings.healthChecks,
	}

	srv := httptest.NewServer(router.New(deps))
	t.Cleanup(func() {
		hub.Close()
		srv.Close()
	})
	return &testEnv{t: t, srv: srv, hub: hub, store: store}
}

func mustSignToken(t *testing.T, secret string, userID int64) string {
	t.Helper()
	tok, err := token.Generate(secret, userID, time.Hour)
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

	res, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := apiResp{Status: res.StatusCode, Header: res.Header, Raw: raw}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &out.Body); err != nil {
			t.Fatalf("%s %s: response is not a JSON envelope: %v\n%s", req.Method, req.URL.Path, err, raw)
		}
	}
	return out
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

func (e *testEnv) register(t *testing.T, username, email, password string) authData {
	t.Helper()
	r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"username": username, "email": email, "password": password,
	})
	expectStatus(t, r, http.StatusCreated)
	var a authData
	decodeData(t, r, &a)
	if a.AccessToken == "" || a.User.ID == 0 {
		t.Fatalf("register returned incomplete data: %s", r.Raw)
	}
	return a
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
		if !r.Body.Success {
			t.Fatalf("success = false: %s", r.Raw)
		}
		var d healthData
		decodeData(t, r, &d)
		if d.Status != "ok" || d.Services["postgres"] != "up" || d.Services["redis"] != "up" {
			t.Fatalf("unexpected health data: %+v", d)
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
		if r.Body.Success {
			t.Fatalf("success = true, want false: %s", r.Raw)
		}
		var d healthData
		decodeData(t, r, &d)
		if d.Status != "degraded" || d.Services["redis"] != "down" || d.Services["postgres"] != "up" {
			t.Fatalf("unexpected health data: %+v", d)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Register -> Login -> Me
// ---------------------------------------------------------------------------

func TestAuthFlow(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)

	// Register (email is trimmed and lower-cased).
	r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"username": "alice", "email": "  Alice@Example.com ", "password": testPass,
	})
	expectStatus(t, r, http.StatusCreated)
	assertNoPasswordLeak(t, r)
	var reg authData
	decodeData(t, r, &reg)
	if reg.AccessToken == "" {
		t.Fatal("register: empty access_token")
	}
	if reg.User.Email != "alice@example.com" || reg.User.Username != "alice" {
		t.Fatalf("register: unexpected user %+v", reg.User)
	}
	if reg.ExpiresIn != int64(time.Hour.Seconds()) {
		t.Fatalf("register: expires_in = %d, want 3600", reg.ExpiresIn)
	}

	// Login (case-insensitive email).
	r = e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email": "ALICE@example.com", "password": testPass,
	})
	expectStatus(t, r, http.StatusOK)
	assertNoPasswordLeak(t, r)
	var login authData
	decodeData(t, r, &login)
	if login.AccessToken == "" || login.User.ID != reg.User.ID {
		t.Fatalf("login: unexpected data %s", r.Raw)
	}

	// /users/me works with both tokens.
	for name, tok := range map[string]string{"register token": reg.AccessToken, "login token": login.AccessToken} {
		r = e.do(t, http.MethodGet, "/api/v1/users/me", tok, nil)
		expectStatus(t, r, http.StatusOK)
		assertNoPasswordLeak(t, r)
		var me model.User
		decodeData(t, r, &me)
		if me.Email != "alice@example.com" || me.ID != reg.User.ID {
			t.Fatalf("me (%s): unexpected user %+v", name, me)
		}
	}

	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/users/999", login.AccessToken, nil), http.StatusNotFound)
	expectStatus(t, e.do(t, http.MethodGet, "/api/v1/users/abc", login.AccessToken, nil), http.StatusBadRequest)
}

// Other users' emails are private: only /users/me (or your own id) shows them.
func TestUserEmailPrivacy(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "alice", "alice@example.com", testPass)
	b := e.register(t, "bobby", "bob@example.com", testPass)

	// hasEmailField reports whether the raw JSON object carries an "email" key.
	hasEmailField := func(t *testing.T, raw json.RawMessage) bool {
		t.Helper()
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("decode user object: %v (%s)", err, raw)
		}
		_, ok := obj["email"]
		return ok
	}

	t.Run("other user by id has no email", func(t *testing.T) {
		r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/users/%d", b.User.ID), a.AccessToken, nil)
		expectStatus(t, r, http.StatusOK)
		assertNoPasswordLeak(t, r)
		if hasEmailField(t, r.Body.Data) {
			t.Fatalf("other user's email exposed: %s", r.Raw)
		}
		if bytes.Contains(r.Raw, []byte("bob@example.com")) {
			t.Fatalf("other user's email exposed: %s", r.Raw)
		}
	})

	t.Run("own user by id has email", func(t *testing.T) {
		r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/users/%d", a.User.ID), a.AccessToken, nil)
		expectStatus(t, r, http.StatusOK)
		var u model.User
		decodeData(t, r, &u)
		if u.Email != "alice@example.com" {
			t.Fatalf("own email = %q, want alice@example.com", u.Email)
		}
	})

	t.Run("list shows only own email", func(t *testing.T) {
		r := e.do(t, http.MethodGet, "/api/v1/users", a.AccessToken, nil)
		expectStatus(t, r, http.StatusOK)
		assertNoPasswordLeak(t, r)
		var items []json.RawMessage
		decodeData(t, r, &items)
		if len(items) != 2 {
			t.Fatalf("users list length = %d, want 2: %s", len(items), r.Raw)
		}
		for _, raw := range items {
			var u model.User
			if err := json.Unmarshal(raw, &u); err != nil {
				t.Fatal(err)
			}
			switch u.ID {
			case a.User.ID:
				if u.Email != "alice@example.com" {
					t.Fatalf("own email in list = %q", u.Email)
				}
			case b.User.ID:
				if hasEmailField(t, raw) {
					t.Fatalf("other user's email exposed in list: %s", raw)
				}
			default:
				t.Fatalf("unexpected user in list: %s", raw)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Register validation
// ---------------------------------------------------------------------------

func TestRegisterValidation(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)

	cases := []struct {
		name string
		body any
	}{
		{"short password", map[string]string{"username": "bob", "email": "bob@example.com", "password": "short"}},
		{"password over 72 bytes", map[string]string{"username": "bob", "email": "bob@example.com", "password": strings.Repeat("x", 73)}},
		{"bad email", map[string]string{"username": "bob", "email": "not-an-email", "password": testPass}},
		{"display-name email", map[string]string{"username": "bob", "email": "Bob <bob@example.com>", "password": testPass}},
		{"empty email", map[string]string{"username": "bob", "email": "", "password": testPass}},
		{"short username", map[string]string{"username": "ab", "email": "bob@example.com", "password": testPass}},
		{"blank username", map[string]string{"username": "     ", "email": "bob@example.com", "password": testPass}},
		{"malformed JSON", `{"username": "bob",`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", tc.body)
			expectStatus(t, r, http.StatusBadRequest)
			if r.Body.Success || r.Body.Error == "" {
				t.Fatalf("expected error envelope, got %s", r.Raw)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4. Duplicate registration
// ---------------------------------------------------------------------------

func TestRegisterDuplicate(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	e.register(t, "carol", "carol@example.com", testPass)

	t.Run("same email different case", func(t *testing.T) {
		r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
			"username": "carol2", "email": "CAROL@example.com", "password": testPass,
		})
		expectStatus(t, r, http.StatusConflict)
	})
	t.Run("same username", func(t *testing.T) {
		r := e.do(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
			"username": "carol", "email": "other@example.com", "password": testPass,
		})
		expectStatus(t, r, http.StatusConflict)
	})
}

// ---------------------------------------------------------------------------
// 5. Bad credentials
// ---------------------------------------------------------------------------

func TestLoginInvalidCredentials(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	e.register(t, "dave", "dave@example.com", testPass)

	cases := []struct{ name, email, password string }{
		{"wrong password", "dave@example.com", "wrong-password"},
		{"unknown email", "nobody@example.com", testPass},
		{"empty password", "dave@example.com", ""},
	}
	var errMsgs []string
	for _, tc := range cases {
		r := e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email": tc.email, "password": tc.password,
		})
		if r.Status != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, want 401; body %s", tc.name, r.Status, r.Raw)
		}
		errMsgs = append(errMsgs, r.Body.Error)
	}
	// No user enumeration: wrong password and unknown email look the same.
	if errMsgs[0] != errMsgs[1] {
		t.Fatalf("error messages differ (user enumeration): %q vs %q", errMsgs[0], errMsgs[1])
	}
}

// ---------------------------------------------------------------------------
// 6. Protected routes require a valid token
// ---------------------------------------------------------------------------

func TestProtectedRoutesRequireToken(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/users/me"},
		{http.MethodGet, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users/1"},
		{http.MethodPost, "/api/v1/messages"},
		{http.MethodGet, "/api/v1/messages/1"},
		{http.MethodGet, "/ws"},
	}
	tokens := []struct{ name, tok string }{
		{"no token", ""},
		{"garbage token", "not.a.jwt"},
		{"wrong secret", mustSignToken(t, "other-secret", 1)},
	}
	for _, rt := range routes {
		for _, tk := range tokens {
			t.Run(rt.method+" "+rt.path+" "+tk.name, func(t *testing.T) {
				t.Parallel()
				var body any
				if rt.method == http.MethodPost {
					body = map[string]any{"receiver_id": 2, "content": "hi"}
				}
				r := e.do(t, rt.method, rt.path, tk.tok, body)
				expectStatus(t, r, http.StatusUnauthorized)
				if r.Body.Success {
					t.Fatalf("success = true on 401: %s", r.Raw)
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// 7. Messages
// ---------------------------------------------------------------------------

func TestMessages(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	a := e.register(t, "alice", "alice@example.com", testPass)
	b := e.register(t, "bobby", "bob@example.com", testPass)
	c := e.register(t, "carol", "carol@example.com", testPass)

	send := func(t *testing.T, tok string, receiver int64, content string) apiResp {
		t.Helper()
		return e.do(t, http.MethodPost, "/api/v1/messages", tok, map[string]any{
			"receiver_id": receiver, "content": content,
		})
	}

	// A -> B (content is trimmed).
	r := send(t, a.AccessToken, b.User.ID, "  hello bob  ")
	expectStatus(t, r, http.StatusCreated)
	var m1 model.Message
	decodeData(t, r, &m1)
	if m1.ID == 0 || m1.SenderID != a.User.ID || m1.ReceiverID != b.User.ID || m1.Content != "hello bob" {
		t.Fatalf("unexpected message: %+v", m1)
	}

	// B -> A, and an unrelated A -> C message.
	expectStatus(t, send(t, b.AccessToken, a.User.ID, "hi alice"), http.StatusCreated)
	expectStatus(t, send(t, a.AccessToken, c.User.ID, "hey carol"), http.StatusCreated)

	// A reads the conversation with B: newest first, excludes A -> C.
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d", b.User.ID), a.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	var conv []model.Message
	decodeData(t, r, &conv)
	if len(conv) != 2 {
		t.Fatalf("conversation length = %d, want 2: %s", len(conv), r.Raw)
	}
	if conv[0].Content != "hi alice" || conv[1].Content != "hello bob" || conv[1].ID != m1.ID {
		t.Fatalf("conversation not newest-first: %+v", conv)
	}

	// B sees the same conversation from the other side.
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d", a.User.ID), b.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	conv = nil
	decodeData(t, r, &conv)
	if len(conv) != 2 {
		t.Fatalf("B's conversation length = %d, want 2", len(conv))
	}

	// Pagination.
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d?limit=1&offset=1", b.User.ID), a.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	conv = nil
	decodeData(t, r, &conv)
	if len(conv) != 1 || conv[0].ID != m1.ID {
		t.Fatalf("paginated conversation = %+v, want only the oldest message", conv)
	}

	// An empty conversation is [] rather than null.
	r = e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/messages/%d", c.User.ID), b.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	if got := strings.TrimSpace(string(r.Body.Data)); got != "[]" {
		t.Fatalf("empty conversation data = %s, want []", got)
	}

	// Error cases.
	t.Run("send to self", func(t *testing.T) {
		expectStatus(t, send(t, a.AccessToken, a.User.ID, "me"), http.StatusBadRequest)
	})
	t.Run("send to nonexistent user", func(t *testing.T) {
		expectStatus(t, send(t, a.AccessToken, 9999, "anyone?"), http.StatusNotFound)
	})
	t.Run("empty content", func(t *testing.T) {
		expectStatus(t, send(t, a.AccessToken, b.User.ID, ""), http.StatusBadRequest)
	})
	t.Run("whitespace content", func(t *testing.T) {
		expectStatus(t, send(t, a.AccessToken, b.User.ID, "   \n\t "), http.StatusBadRequest)
	})
	t.Run("content too long", func(t *testing.T) {
		expectStatus(t, send(t, a.AccessToken, b.User.ID, strings.Repeat("a", 2001)), http.StatusBadRequest)
	})
	t.Run("invalid receiver id", func(t *testing.T) {
		expectStatus(t, send(t, a.AccessToken, 0, "x"), http.StatusBadRequest)
	})
	t.Run("malformed JSON", func(t *testing.T) {
		expectStatus(t, e.do(t, http.MethodPost, "/api/v1/messages", a.AccessToken, `{"receiver_id":`), http.StatusBadRequest)
	})
	t.Run("invalid path id", func(t *testing.T) {
		expectStatus(t, e.do(t, http.MethodGet, "/api/v1/messages/abc", a.AccessToken, nil), http.StatusBadRequest)
	})
}

// ---------------------------------------------------------------------------
// 8. WebSocket
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
	a := e.register(t, "alice", "alice@example.com", testPass)
	b := e.register(t, "bobby", "bob@example.com", testPass)

	conn, res, err := dialWS(e.wsURL(b.AccessToken), allowedOrig)
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	defer conn.Close()

	// Reader goroutine forwards every event to a channel. A single read
	// deadline bounds the whole test (gorilla conns are unusable after a
	// read timeout, so per-read deadlines are not used).
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

	// The hub registers the client just after the handshake response is
	// written, so push sync events directly via the hub until one arrives;
	// only then is the connection guaranteed to be registered.
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
	b := e.register(t, "bobby", "bob@example.com", testPass)

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
// 9. CORS
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
		if got := strings.ToLower(r.Header.Get("Access-Control-Allow-Headers")); !strings.Contains(got, "authorization") || !strings.Contains(got, "content-type") {
			t.Fatalf("Allow-Headers = %q", got)
		}
	})

	t.Run("disallowed origin preflight", func(t *testing.T) {
		t.Parallel()
		r := preflight(t, "http://evil.example")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("Allow-Origin = %q, want none", got)
		}
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("Allow-Credentials = %q, want none", got)
		}
	})

	t.Run("wildcard origin omits credentials", func(t *testing.T) {
		t.Parallel()
		w := newTestEnv(t, withCORSOrigins("*"))
		req, err := http.NewRequest(http.MethodOptions, w.srv.URL+"/api/v1/auth/login", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "http://anything.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		r := w.send(t, req)
		expectStatus(t, r, http.StatusNoContent)
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "http://anything.example" {
			t.Fatalf("Allow-Origin = %q, want echoed origin", got)
		}
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("Allow-Credentials = %q, want none for wildcard config", got)
		}
	})

	t.Run("simple request from allowed origin", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", allowedOrig)
		r := e.send(t, req)
		expectStatus(t, r, http.StatusOK)
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != allowedOrig {
			t.Fatalf("Allow-Origin = %q, want %q", got, allowedOrig)
		}
		if !strings.Contains(strings.Join(r.Header.Values("Vary"), ","), "Origin") {
			t.Fatalf("Vary header missing Origin: %v", r.Header.Values("Vary"))
		}
	})
}

// ---------------------------------------------------------------------------
// 10. Rate limiting
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
	if last.Body.Success || last.Body.Error == "" {
		t.Fatalf("429 body is not an error envelope: %s", last.Raw)
	}
}
