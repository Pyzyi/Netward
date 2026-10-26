// Package api serves the REST API, the live event stream and the web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/auth"
	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/fritzbox"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/internet"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/mqtt"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/scanner"
	"github.com/Pyzyi/netzwart/internal/security"
	"github.com/Pyzyi/netzwart/internal/store"
)

// SetupWindow is how long after start the initial account can be created.
const SetupWindow = 30 * time.Minute

// Deps are the components the API talks to.
type Deps struct {
	Store     *store.Store
	Monitor   *monitor.Monitor
	Scanner   *scanner.Scanner // nil in demo mode
	Internet  *internet.Monitor
	Security  *security.Checker
	Notify    *notify.Dispatcher
	MQTT      *mqtt.Bridge
	FritzBox  *fritzbox.Poller
	Hub       *hub.Hub
	Env       config.Env
	Log       *slog.Logger
	Version   string
	StartedAt time.Time
	Static    fs.FS // built web UI; may be nil

	Settings      func() config.Settings
	ApplySettings func(context.Context, config.Settings) error
	LocalPrefixes func() []netip.Prefix // directly connected networks (setup check)
	RawSockets    bool
}

// Server is the HTTP API.
type Server struct {
	d       Deps
	limiter *auth.Limiter

	trustMu sync.Mutex
	trusted map[netip.Addr]time.Time // addresses with a successful login
	streams *streamRegistry
	static  *staticFiles
	now     func() time.Time

	setupMu sync.Mutex
}

// New creates the server.
func New(d Deps) *Server {
	s := &Server{d: d, limiter: auth.NewLimiter(), streams: newStreamRegistry(), now: time.Now}
	if d.Static != nil {
		s.static = newStaticFiles(d.Static)
	}
	return s
}

// apiError is the JSON error body. Code is stable and translated by the UI.
type apiError struct {
	Code    string `json:"error"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Code: code, Message: msg})
}

const maxBody = 1 << 20

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be a single object")
		return false
	}
	return true
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// Handler returns the complete HTTP handler with all middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})

	// Public endpoints.
	mux.HandleFunc("GET /api/v1/auth/state", s.handleAuthState)
	mux.HandleFunc("POST /api/v1/auth/setup", s.handleSetup)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)

	// Authenticated endpoints.
	authed := func(pattern string, h func(http.ResponseWriter, *http.Request, *sessionInfo)) {
		mux.Handle(pattern, s.requireSession(h))
	}
	authed("POST /api/v1/auth/logout", s.handleLogout)
	authed("POST /api/v1/auth/password", s.handlePassword)
	authed("GET /api/v1/auth/sessions", s.handleSessions)
	authed("DELETE /api/v1/auth/sessions/{id}", s.handleRevokeSession)
	authed("POST /api/v1/auth/sessions/revoke-others", s.handleRevokeOtherSessions)

	authed("GET /api/v1/overview", s.handleOverview)
	authed("GET /api/v1/devices", s.handleDevices)
	authed("POST /api/v1/devices/acknowledge", s.handleAcknowledge)
	authed("GET /api/v1/devices/{id}", s.handleDevice)
	authed("PATCH /api/v1/devices/{id}", s.handlePatchDevice)
	authed("DELETE /api/v1/devices/{id}", s.handleDeleteDevice)
	authed("POST /api/v1/devices/{id}/merge", s.handleMerge)
	authed("GET /api/v1/devices/{id}/presence", s.handlePresence)
	authed("GET /api/v1/presence/strips", s.handlePresenceStrips)
	authed("GET /api/v1/devices/{id}/events", s.handleDeviceEvents)
	authed("POST /api/v1/devices/{id}/wake", s.handleWake)
	authed("POST /api/v1/devices/{id}/ping", s.handlePing)
	authed("POST /api/v1/devices/{id}/portscan", s.handlePortScan)
	authed("GET /api/v1/events", s.handleEvents)
	authed("POST /api/v1/scan", s.handleScan)
	authed("GET /api/v1/settings", s.handleGetSettings)
	authed("PUT /api/v1/settings", s.handlePutSettings)
	authed("GET /api/v1/channels", s.handleChannels)
	authed("POST /api/v1/channels", s.handleCreateChannel)
	authed("PUT /api/v1/channels/{id}", s.handleUpdateChannel)
	authed("DELETE /api/v1/channels/{id}", s.handleDeleteChannel)
	authed("POST /api/v1/channels/{id}/test", s.handleTestChannel)
	authed("GET /api/v1/security", s.handleSecurity)
	authed("POST /api/v1/security/run", s.handleSecurityRun)
	authed("POST /api/v1/security/findings/{id}/dismiss", s.handleDismissFinding)
	authed("GET /api/v1/internet", s.handleInternet)
	authed("GET /api/v1/export", s.handleExport)
	authed("GET /api/v1/system", s.handleSystem)
	authed("GET /api/v1/stream", s.handleStream)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "")
	})
	if s.static != nil {
		mux.Handle("/", s.static)
	}
	return s.recoverer(s.securityHeaders(s.checkHost(s.checkOrigin(mux))))
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.d.Log.Error("panic in handler", "path", r.URL.Path, "panic", v)
				writeError(w, http.StatusInternalServerError, "internal", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CSP allows only same-origin scripts. Inline styles stay allowed because
// Svelte sets style attributes; script injection remains impossible.
const csp = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if s.isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// localSuffixes are domains that cannot be registered publicly, so an
// attacker cannot point them at the victim's network via DNS rebinding.
// (".box" itself is a public TLD; only AVM controls fritz.box.)
var localSuffixes = []string{".local", ".lan", ".home", ".home.arpa", ".internal", ".intranet", ".localdomain",
	".fritz.box", ".speedport.ip", ".router", ".localhost"}

// HostAllowed reports whether a Host header value is acceptable.
func HostAllowed(hostport string, extra []string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if host == "" {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return true
	}
	for _, s := range localSuffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	for _, e := range extra {
		if host == e || (strings.HasPrefix(e, "*.") && strings.HasSuffix(host, e[1:])) {
			return true
		}
	}
	return false
}

func (s *Server) checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !HostAllowed(r.Host, s.d.Env.AllowedHosts) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusMisdirectedRequest)
			_, _ = io.WriteString(w, "This host name is not allowed. If you use a reverse proxy, add it to NETZWART_ALLOWED_HOSTS.\n")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func normHostPort(host string, https bool) string {
	host = strings.ToLower(host)
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if https {
		return net.JoinHostPort(strings.Trim(host, "[]"), "443")
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), "80")
}

// checkOrigin requires an Origin header on unsafe requests that matches the
// request's own origin or an explicitly allowed host (reverse proxies that
// rewrite Host). Combined with the Host allowlist this defeats CSRF and
// DNS rebinding, including for setup and login which have no session yet.
func (s *Server) checkOrigin(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	inner := cop.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		u, err := url.Parse(origin)
		if origin == "" || origin == "null" || err != nil || u.Host == "" {
			writeError(w, http.StatusForbidden, "origin_required", "")
			return
		}
		ok := normHostPort(u.Host, u.Scheme == "https") == normHostPort(r.Host, s.isHTTPS(r))
		if !ok {
			oh := strings.ToLower(u.Hostname())
			for _, e := range s.d.Env.AllowedHosts {
				if oh == e || (strings.HasPrefix(e, "*.") && strings.HasSuffix(oh, e[1:])) {
					ok = true
					break
				}
			}
		}
		if !ok {
			writeError(w, http.StatusForbidden, "origin_mismatch", "")
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func (s *Server) trustedProxy(ip netip.Addr) bool {
	for _, p := range s.d.Env.TrustedProxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteIP(r *http.Request) netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	return ap.Addr().Unmap()
}

// clientIP returns the client address, honouring X-Forwarded-For only from
// trusted proxies (rightmost untrusted entry wins).
func (s *Server) clientIP(r *http.Request) netip.Addr {
	ip := remoteIP(r)
	if !s.trustedProxy(ip) {
		return ip
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		if !s.trustedProxy(a) {
			return a
		}
		ip = a
	}
	return ip
}

func (s *Server) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return s.trustedProxy(remoteIP(r)) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func errorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, monitor.ErrInvalid):
		return http.StatusBadRequest, "invalid_input"
	case errors.Is(err, context.Canceled):
		return 499, "canceled"
	}
	return http.StatusInternalServerError, "internal"
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := errorStatus(err)
	if status == http.StatusInternalServerError {
		s.d.Log.Error("request failed", "path", r.URL.Path, "err", err)
	}
	writeError(w, status, code, "")
}
