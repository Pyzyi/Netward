package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/internet"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/security"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
)

const testHost = "192.168.1.5:8840"
const testOrigin = "http://192.168.1.5:8840"

type harness struct {
	t   *testing.T
	srv *Server
	h   http.Handler
	st  *store.Store
	mon *monitor.Monitor

	mu       sync.Mutex
	settings config.Settings
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	disp := notify.NewDispatcher(st, &egress.Policy{}, log)
	inet := internet.New(st, h, mon, disp, nil, &egress.Policy{}, func() []netip.Addr { return nil }, log)
	hs := &harness{t: t, st: st, mon: mon, settings: config.Defaults()}
	hs.srv = New(Deps{
		Store: st, Monitor: mon, Internet: inet, Security: security.New(st, mon, h, disp, log), Notify: disp, Hub: h,
		Log: log, Version: "test", StartedAt: time.Now(),
		Static:   fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Netzwart</title>")}},
		Settings: func() config.Settings { hs.mu.Lock(); defer hs.mu.Unlock(); return hs.settings },
		ApplySettings: func(_ context.Context, s config.Settings) error {
			hs.mu.Lock()
			hs.settings = s
			hs.mu.Unlock()
			return nil
		},
		LocalPrefixes: func() []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")} },
	})
	hs.h = hs.srv.Handler()
	return hs
}

type req struct {
	method, path string
	body         any
	host         string
	origin       string
	remote       string
	cookie       *http.Cookie
	csrf         string
	noOrigin     bool
}

func (h *harness) do(r req) *httptest.ResponseRecorder {
	h.t.Helper()
	var body io.Reader
	if r.body != nil {
		b, _ := json.Marshal(r.body)
		body = bytes.NewReader(b)
	}
	hr := httptest.NewRequest(r.method, r.path, body)
	hr.Host = testHost
	if r.host != "" {
		hr.Host = r.host
	}
	hr.RemoteAddr = "192.168.1.20:50000"
	if r.remote != "" {
		hr.RemoteAddr = r.remote
	}
	if r.body != nil {
		hr.Header.Set("Content-Type", "application/json")
	}
	if !r.noOrigin && r.method != http.MethodGet {
		o := testOrigin
		if r.origin != "" {
			o = r.origin
		}
		hr.Header.Set("Origin", o)
	}
	if r.cookie != nil {
		hr.AddCookie(r.cookie)
	}
	if r.csrf != "" {
		hr.Header.Set("X-CSRF-Token", r.csrf)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, hr)
	return rec
}

type session struct {
	cookie *http.Cookie
	csrf   string
}

func (h *harness) setup() session {
	h.t.Helper()
	rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: map[string]any{
		"username": "admin", "password": "Sonnenblume-im-Garten7", "language": "de", "timezone": "Europe/Berlin"}})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	var st stateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	var c *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "netzwart_session" {
			c = ck
		}
	}
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		h.t.Fatalf("session cookie missing or insecure: %+v", c)
	}
	return session{cookie: c, csrf: st.CSRF}
}

func TestSecurityHeaders(t *testing.T) {
	h := newHarness(t)
	rec := h.do(req{method: "GET", path: "/"})
	for k, want := range map[string]string{
		"Content-Security-Policy": "default-src 'none'; script-src 'self';",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	} {
		if got := rec.Header().Get(k); !strings.HasPrefix(got, want) {
			t.Errorf("%s = %q, want prefix %q", k, got, want)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("frame-ancestors missing")
	}
	if !strings.Contains(rec.Body.String(), "Netzwart") {
		t.Error("index not served")
	}
	// SPA fallback for client routes, 404 for missing assets.
	if rec := h.do(req{method: "GET", path: "/devices/12"}); rec.Code != 200 {
		t.Errorf("SPA fallback: %d", rec.Code)
	}
	if rec := h.do(req{method: "GET", path: "/assets/missing.js"}); rec.Code != 404 {
		t.Errorf("missing asset: %d", rec.Code)
	}
}

func TestDNSRebindingBlocked(t *testing.T) {
	h := newHarness(t)
	for _, host := range []string{"evil.example:8840", "attacker.com", "rebind.it.", "evil.box", "nas.box:8840"} {
		rec := h.do(req{method: "GET", path: "/api/v1/auth/state", host: host})
		if rec.Code != http.StatusMisdirectedRequest {
			t.Errorf("Host %q: got %d, want 421", host, rec.Code)
		}
		rec = h.do(req{method: "POST", path: "/api/v1/auth/setup", host: host, origin: "http://" + host,
			body: map[string]any{"username": "evil", "password": "Sonnenblume-im-Garten7"}})
		if rec.Code != http.StatusMisdirectedRequest {
			t.Errorf("setup via Host %q: got %d", host, rec.Code)
		}
	}
	for _, host := range []string{"192.168.1.5:8840", "zimaos:8840", "zimaos.local", "localhost:8840", "[fe80::1]:8840", "nas.fritz.box"} {
		if !HostAllowed(host, nil) {
			t.Errorf("%q should be allowed", host)
		}
	}
	if !HostAllowed("netzwart.example.com", []string{"netzwart.example.com"}) || !HostAllowed("a.example.com", []string{"*.example.com"}) {
		t.Error("configured hosts not allowed")
	}
}

func TestSetupProtections(t *testing.T) {
	h := newHarness(t)
	body := map[string]any{"username": "admin", "password": "Sonnenblume-im-Garten7"}
	// Cross-origin (an IP-literal web page) and missing Origin are refused.
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: body, origin: "http://203.0.113.9"}); rec.Code != 403 {
		t.Fatalf("cross-origin setup: %d", rec.Code)
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: body, noOrigin: true}); rec.Code != 403 {
		t.Fatalf("setup without Origin: %d", rec.Code)
	}
	// Not from a directly connected network (X-Forwarded-For is ignored).
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: body, remote: "203.0.113.9:4000"}); rec.Code != 403 {
		t.Fatalf("remote setup: %d", rec.Code)
	}
	// Weak password.
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: map[string]any{"username": "admin", "password": "1234567890"}}); rec.Code != 400 {
		t.Fatalf("weak password: %d", rec.Code)
	}
	h.setup()
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: body}); rec.Code != http.StatusConflict {
		t.Fatalf("second setup: %d", rec.Code)
	}
	if h.settings.Timezone != "Europe/Berlin" {
		t.Fatal("browser time zone not stored")
	}
}

func TestSetupWindowExpires(t *testing.T) {
	h := newHarness(t)
	h.srv.now = func() time.Time { return time.Now().Add(SetupWindow + time.Minute) }
	rec := h.do(req{method: "POST", path: "/api/v1/auth/setup", body: map[string]any{"username": "admin", "password": "Sonnenblume-im-Garten7"}})
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "setup_expired") {
		t.Fatalf("expired setup: %d %s", rec.Code, rec.Body)
	}
}

func TestAuthRequiredAndCSRF(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	for _, p := range []string{"/api/v1/overview", "/api/v1/devices", "/api/v1/settings", "/api/v1/channels", "/api/v1/events",
		"/api/v1/export", "/api/v1/system", "/api/v1/stream", "/api/v1/security", "/api/v1/internet"} {
		if rec := h.do(req{method: "GET", path: p}); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without session: %d", p, rec.Code)
		}
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/overview", cookie: s.cookie}); rec.Code != 200 {
		t.Fatalf("overview with session: %d", rec.Code)
	}
	// Unsafe request without / with wrong CSRF token.
	if rec := h.do(req{method: "POST", path: "/api/v1/devices/acknowledge", cookie: s.cookie}); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/devices/acknowledge", cookie: s.cookie, csrf: "wrong"}); rec.Code != 403 {
		t.Fatalf("wrong CSRF: %d", rec.Code)
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/devices/acknowledge", cookie: s.cookie, csrf: s.csrf}); rec.Code != 200 {
		t.Fatalf("valid CSRF: %d %s", rec.Code, rec.Body)
	}
	// Valid CSRF but cross-origin.
	if rec := h.do(req{method: "POST", path: "/api/v1/devices/acknowledge", cookie: s.cookie, csrf: s.csrf, origin: "http://192.168.1.99:8080"}); rec.Code != 403 {
		t.Fatalf("cross-origin with token: %d", rec.Code)
	}
	// Logout invalidates the session server-side.
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/logout", cookie: s.cookie, csrf: s.csrf}); rec.Code != 204 {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/overview", cookie: s.cookie}); rec.Code != 401 {
		t.Fatalf("session still valid after logout: %d", rec.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t)
	h.setup()
	bad := map[string]any{"username": "admin", "password": "falsch-falsch-falsch"}
	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		last = h.do(req{method: "POST", path: "/api/v1/auth/login", body: bad})
	}
	if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
		t.Fatalf("6th failed login: %d", last.Code)
	}
	// Unknown users get the same answer as wrong passwords.
	h2 := newHarness(t)
	h2.setup()
	a := h2.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]any{"username": "nobody", "password": "x"}})
	b := h2.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]any{"username": "admin", "password": "x"}})
	if a.Code != b.Code || a.Body.String() != b.Body.String() {
		t.Fatalf("user enumeration possible: %d %s vs %d %s", a.Code, a.Body, b.Code, b.Body)
	}
	ok := h2.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]any{"username": "ADMIN", "password": "Sonnenblume-im-Garten7"}})
	if ok.Code != 200 {
		t.Fatalf("valid login: %d", ok.Code)
	}
}

func TestChannelSecretsAreWriteOnly(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	create := h.do(req{method: "POST", path: "/api/v1/channels", cookie: s.cookie, csrf: s.csrf, body: map[string]any{
		"name": "Telegram", "kind": "telegram", "enabled": true, "events": []string{"device_new", "bogus"},
		"config": map[string]any{"token": "123:SECRET-BOT-TOKEN", "chatId": "42"}}})
	if create.Code != http.StatusCreated {
		t.Fatalf("create channel: %d %s", create.Code, create.Body)
	}
	if strings.Contains(create.Body.String(), "SECRET") || strings.Contains(create.Body.String(), "bogus") {
		t.Fatalf("secret or invalid event in response: %s", create.Body)
	}
	list := h.do(req{method: "GET", path: "/api/v1/channels", cookie: s.cookie})
	if strings.Contains(list.Body.String(), "SECRET") {
		t.Fatal("secret returned by GET")
	}
	var created store.Channel
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	// Updating with the masked value keeps the secret.
	upd := h.do(req{method: "PUT", path: "/api/v1/channels/" + itoa(created.ID), cookie: s.cookie, csrf: s.csrf, body: map[string]any{
		"name": "Telegram Familie", "kind": "telegram", "enabled": true, "events": []string{"device_new"},
		"config": map[string]any{"token": "••••••••", "chatId": "42"}}})
	if upd.Code != 200 {
		t.Fatalf("update: %d %s", upd.Code, upd.Body)
	}
	ch, _ := h.st.GetChannel(context.Background(), created.ID)
	if !strings.Contains(string(ch.Config), "SECRET-BOT-TOKEN") {
		t.Fatal("masked update overwrote the secret")
	}
	// Settings secrets likewise.
	st := h.settings
	st.MQTT = config.MQTT{Enabled: true, Host: "192.168.1.10", Port: 1883, Username: "ha", Password: "mqtt-secret",
		DiscoveryPrefix: "homeassistant", BaseTopic: "netzwart"}
	if rec := h.do(req{method: "PUT", path: "/api/v1/settings", cookie: s.cookie, csrf: s.csrf, body: st}); rec.Code != 200 || strings.Contains(rec.Body.String(), "mqtt-secret") {
		t.Fatalf("settings update: %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/settings", cookie: s.cookie}); strings.Contains(rec.Body.String(), "mqtt-secret") {
		t.Fatal("MQTT password returned")
	}
	// Pointing MQTT at another broker with the masked password must not hand it over.
	st.MQTT.Host, st.MQTT.Password = "192.0.2.2", "••••••••"
	if rec := h.do(req{method: "PUT", path: "/api/v1/settings", cookie: s.cookie, csrf: s.csrf, body: st}); rec.Code != 400 || !strings.Contains(rec.Body.String(), "secret_required") {
		t.Fatalf("MQTT password followed a new host: %d %s", rec.Code, rec.Body)
	}
	// A channel changed to another kind never inherits the old secrets.
	conv := h.do(req{method: "PUT", path: "/api/v1/channels/" + itoa(created.ID), cookie: s.cookie, csrf: s.csrf, body: map[string]any{
		"name": "x", "kind": "gotify", "enabled": true, "events": []string{"device_new"},
		"config": map[string]any{"server": "https://evil.example", "token": "••••••••"}}})
	if conv.Code == 200 {
		ch, _ := h.st.GetChannel(context.Background(), created.ID)
		if strings.Contains(string(ch.Config), "SECRET") {
			t.Fatal("secret carried over to another channel kind")
		}
	}
}

func TestInvalidInput(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	bad := h.settings
	bad.ScanInterval = 5
	if rec := h.do(req{method: "PUT", path: "/api/v1/settings", cookie: s.cookie, csrf: s.csrf, body: bad}); rec.Code != 400 {
		t.Fatalf("invalid interval accepted: %d", rec.Code)
	}
	if rec := h.do(req{method: "PATCH", path: "/api/v1/devices/abc", cookie: s.cookie, csrf: s.csrf, body: map[string]any{}}); rec.Code != 400 {
		t.Fatalf("bad id: %d", rec.Code)
	}
	if rec := h.do(req{method: "PATCH", path: "/api/v1/devices/999", cookie: s.cookie, csrf: s.csrf, body: map[string]any{"bogus": 1}}); rec.Code != 400 {
		t.Fatalf("unknown field: %d", rec.Code)
	}
	huge := strings.Repeat("x", 2<<20)
	if rec := h.do(req{method: "PATCH", path: "/api/v1/devices/1", cookie: s.cookie, csrf: s.csrf, body: map[string]any{"notes": huge}}); rec.Code != 400 {
		t.Fatalf("oversized body: %d", rec.Code)
	}
}

func TestDeviceFlowAndCSVExport(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	mac := []byte{0x3c, 0x22, 0xfb, 0, 0, 7}
	h.mon.ApplySweep(context.Background(), "eth0", l2.SweepResult{netip.MustParseAddr("192.168.1.7"): {mac}}, time.Now())
	var devs []store.Device
	_ = json.Unmarshal(h.do(req{method: "GET", path: "/api/v1/devices", cookie: s.cookie}).Body.Bytes(), &devs)
	if len(devs) != 1 {
		t.Fatalf("devices: %+v", devs)
	}
	name := "=HYPERLINK(\"http://evil\")"
	rec := h.do(req{method: "PATCH", path: "/api/v1/devices/" + itoa(devs[0].ID), cookie: s.cookie, csrf: s.csrf, body: map[string]any{"name": name, "watch": true}})
	if rec.Code != 200 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	csvRec := h.do(req{method: "GET", path: "/api/v1/export?format=csv", cookie: s.cookie})
	if !strings.Contains(csvRec.Body.String(), "'=HYPERLINK") {
		t.Fatalf("formula not neutralised:\n%s", csvRec.Body)
	}
	pres := h.do(req{method: "GET", path: "/api/v1/devices/" + itoa(devs[0].ID) + "/presence?range=7d", cookie: s.cookie})
	if pres.Code != 200 || !strings.Contains(pres.Body.String(), `"spans":[{`) {
		t.Fatalf("presence: %d %s", pres.Code, pres.Body)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestPresenceStrips(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	h.mon.Observe(context.Background(), l2.Sighting{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 0, 9}, IP: netip.MustParseAddr("192.168.1.9"),
		Iface: "eth0", Source: "arp", ARP: true, At: time.Now()})
	rec := h.do(req{method: "GET", path: "/api/v1/presence/strips", cookie: s.cookie})
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var r struct {
		Slot   int64             `json:"slot"`
		Strips map[string]string `json:"strips"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	if len(r.Strips) != 1 || r.Slot != 30*60*1000 {
		t.Fatalf("unexpected %+v", r)
	}
	for _, st := range r.Strips {
		if len(st) != 48 || st[47] != '1' || st[0] != '-' {
			t.Fatalf("strip %q: want unknown history and online now", st)
		}
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/presence/strips"}); rec.Code != 401 {
		t.Fatal("strips readable without session")
	}
}

func TestRevokeOtherSessions(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	var others []*http.Cookie
	for i := 0; i < 2; i++ {
		rec := h.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]any{"username": "admin", "password": "Sonnenblume-im-Garten7"}})
		if rec.Code != 200 {
			t.Fatalf("login: %d %s", rec.Code, rec.Body)
		}
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == "netzwart_session" {
				others = append(others, ck)
			}
		}
	}
	if len(others) != 2 {
		t.Fatalf("expected two extra sessions, got %d", len(others))
	}
	if rec := h.do(req{method: "POST", path: "/api/v1/auth/sessions/revoke-others", cookie: s.cookie}); rec.Code != 403 {
		t.Fatalf("revoke-others without CSRF: %d", rec.Code)
	}
	rec := h.do(req{method: "POST", path: "/api/v1/auth/sessions/revoke-others", cookie: s.cookie, csrf: s.csrf})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"revoked":2`) {
		t.Fatalf("revoke-others: %d %s", rec.Code, rec.Body)
	}
	for _, c := range others {
		if rec := h.do(req{method: "GET", path: "/api/v1/overview", cookie: c}); rec.Code != 401 {
			t.Fatalf("other session still valid: %d", rec.Code)
		}
	}
	if rec := h.do(req{method: "GET", path: "/api/v1/overview", cookie: s.cookie}); rec.Code != 200 {
		t.Fatalf("own session revoked: %d", rec.Code)
	}
}

// TestPresenceStripsEdges pins the slot rules: unknown before first sight and
// inside (merged) gaps, a span ending on a slot boundary leaves that slot
// offline, open spans run up to now.
func TestPresenceStripsEdges(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	ctx := context.Background()
	const slot = int64(30 * 60 * 1000)
	now := time.Now()
	to := (now.UnixMilli()/slot + 1) * slot
	from := to - 48*slot
	at := func(i int, plusMin int64) int64 { return from + int64(i)*slot + plusMin*60_000 }

	h.mon.Observe(ctx, l2.Sighting{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 0, 7}, IP: netip.MustParseAddr("192.168.1.7"),
		Iface: "eth0", Source: "arp", ARP: true, At: time.UnixMilli(at(10, 1))})
	devs := h.mon.Devices()
	if len(devs) != 1 {
		t.Fatalf("devices: %d", len(devs))
	}
	id := devs[0].ID
	if err := h.st.ClosePresence(ctx, id, at(20, 0)); err != nil { // exactly on the start of slot 20
		t.Fatal(err)
	}
	if err := h.st.OpenPresence(ctx, id, at(30, 5)); err != nil {
		t.Fatal(err)
	}
	// Two back-to-back gaps covering slots 22–24; they meet inside slot 23,
	// which neither covers alone.
	if err := h.st.AddGap(ctx, at(22, 0), at(23, 15)); err != nil {
		t.Fatal(err)
	}
	if err := h.st.AddGap(ctx, at(23, 15), at(25, 0)); err != nil {
		t.Fatal(err)
	}
	h.srv.now = func() time.Time { return now }

	rec := h.do(req{method: "GET", path: "/api/v1/presence/strips", cookie: s.cookie})
	var r struct {
		From   int64             `json:"from"`
		Strips map[string]string `json:"strips"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil || rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if r.From != from {
		t.Fatalf("from = %d, want %d", r.From, from)
	}
	want := strings.Repeat("-", 10) + strings.Repeat("1", 10) + "00" + "---" + strings.Repeat("0", 5) + strings.Repeat("1", 18)
	if got := r.Strips[itoa(id)]; got != want {
		t.Fatalf("strip\n got %s\nwant %s", got, want)
	}
}

func TestSoftwareFindingsAndManual(t *testing.T) {
	h := newHarness(t)
	s := h.setup()
	ctx := context.Background()
	h.mon.Observe(ctx, l2.Sighting{MAC: net.HardwareAddr{0x04, 0x42, 0x1a, 0, 0, 1}, IP: netip.MustParseAddr("192.168.1.42"),
		Iface: "eth0", Source: "arp", ARP: true, At: time.Now()})
	id := h.mon.Devices()[0].ID
	h.mon.AddSoftware(id, software.FromNTLMVersion(10, 0, 19045))
	sec := h.srv.d.Security
	sec.EvaluateSoftware(ctx)
	fs, _ := h.st.ListFindings(ctx)
	if len(fs) != 1 || fs[0].CheckID != security.CheckSoftwareOutdated || fs[0].DeviceID != id || fs[0].Severity != "warning" {
		t.Fatalf("findings %+v", fs)
	}
	// A different build with the same verdict is the same finding.
	h.mon.AddSoftware(id, software.FromNTLMVersion(10, 0, 19044))
	sec.EvaluateSoftware(ctx)
	if fs2, _ := h.st.ListFindings(ctx); len(fs2) != 1 || fs2[0].ID != fs[0].ID {
		t.Fatalf("version change created a new finding: %+v", fs2)
	}
	// Marking the PC as LTSC makes 19044 supported and clears the finding.
	rec := h.do(req{method: "PUT", path: "/api/v1/devices/" + itoa(id) + "/software", cookie: s.cookie, csrf: s.csrf,
		body: map[string]any{"edition": "extended"}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"supported"`) {
		t.Fatalf("edition: %d %s", rec.Code, rec.Body)
	}
	sec.EvaluateSoftware(ctx)
	if fs3, _ := h.st.ListFindings(ctx); len(fs3) != 0 {
		t.Fatalf("finding not cleared: %+v", fs3)
	}
	// Manual input is validated.
	for _, body := range []map[string]any{
		{"manual": map[string]any{"product": "windows", "version": "11"}},
		{"manual": map[string]any{"product": "ios", "version": "18<script>"}},
		{"edition": "server2025"},
	} {
		if rec := h.do(req{method: "PUT", path: "/api/v1/devices/" + itoa(id) + "/software", cookie: s.cookie, csrf: s.csrf, body: body}); rec.Code != 400 {
			t.Fatalf("accepted %v: %d", body, rec.Code)
		}
	}
	rec = h.do(req{method: "PUT", path: "/api/v1/devices/" + itoa(id) + "/software", cookie: s.cookie, csrf: s.csrf,
		body: map[string]any{"manual": map[string]any{"product": "ios", "version": "16"}}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"source":"manual"`) {
		t.Fatalf("manual: %d %s", rec.Code, rec.Body)
	}
	// The detail view carries the full assessment, the list the summary.
	rec = h.do(req{method: "GET", path: "/api/v1/devices/" + itoa(id), cookie: s.cookie})
	if !strings.Contains(rec.Body.String(), `"software":{"status":"outdated","product":"ios"`) {
		t.Fatalf("detail: %s", rec.Body)
	}
	if rec := h.do(req{method: "PUT", path: "/api/v1/devices/" + itoa(id) + "/software", cookie: s.cookie,
		body: map[string]any{"edition": ""}}); rec.Code != 403 {
		t.Fatalf("missing CSRF accepted: %d", rec.Code)
	}
}
