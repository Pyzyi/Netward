package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/Pyzyi/netzwart/internal/auth"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/textutil"
)

const (
	idleShort   = 7 * 24 * time.Hour
	idleLong    = 30 * 24 * time.Hour
	absoluteMax = 30 * 24 * time.Hour
	touchEvery  = time.Minute
)

type sessionInfo struct {
	session store.Session
	user    store.User
	token   string
}

func (s *Server) cookieName(r *http.Request) string {
	if s.isHTTPS(r) {
		return "__Host-netzwart_session"
	}
	return "netzwart_session"
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string, remember bool) {
	c := &http.Cookie{Name: s.cookieName(r), Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: s.isHTTPS(r)}
	if remember {
		c.MaxAge = int(idleLong / time.Second)
	}
	http.SetCookie(w, c)
}

func (s *Server) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(r), Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: s.isHTTPS(r), MaxAge: -1})
}

// lookupSession resolves the session cookie; it returns nil if there is none.
func (s *Server) lookupSession(r *http.Request) *sessionInfo {
	c, err := r.Cookie(s.cookieName(r))
	if err != nil || len(c.Value) != 43 {
		return nil
	}
	now := s.now()
	se, err := s.d.Store.GetSessionByHash(r.Context(), auth.HashToken(c.Value), now.UnixMilli())
	if err != nil {
		return nil
	}
	if now.Sub(time.UnixMilli(se.CreatedAt)) > absoluteMax {
		_ = s.d.Store.DeleteSession(r.Context(), se.UserID, se.ID)
		return nil
	}
	u, err := s.d.Store.GetUser(r.Context(), se.UserID)
	if err != nil {
		return nil
	}
	if now.Sub(time.UnixMilli(se.LastSeenAt)) > touchEvery {
		idle := idleShort
		if se.Remember {
			idle = idleLong
		}
		_ = s.d.Store.TouchSession(r.Context(), se.ID, now.UnixMilli(), now.Add(idle).UnixMilli(), s.clientIP(r).String())
	}
	return &sessionInfo{session: se, user: u, token: c.Value}
}

func (s *Server) requireSession(h func(http.ResponseWriter, *http.Request, *sessionInfo)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		si := s.lookupSession(r)
		if si == nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "")
			return
		}
		if !safeMethod(r.Method) {
			got := r.Header.Get("X-CSRF-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(si.session.CSRFToken)) != 1 {
				writeError(w, http.StatusForbidden, "csrf", "")
				return
			}
		}
		h(w, r, si)
	})
}

func (s *Server) createSession(ctx context.Context, w http.ResponseWriter, r *http.Request, u store.User, remember bool) (store.Session, error) {
	token := auth.NewToken()
	now := s.now()
	idle := idleShort
	if remember {
		idle = idleLong
	}
	se := store.Session{
		TokenHash: auth.HashToken(token), UserID: u.ID, CSRFToken: auth.NewToken(),
		CreatedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(), ExpiresAt: now.Add(idle).UnixMilli(), Remember: remember,
		UserAgent: textutil.Clean(r.UserAgent(), 200), IP: s.clientIP(r).String(),
	}
	if err := s.d.Store.CreateSession(ctx, &se); err != nil {
		return se, err
	}
	s.setCookie(w, r, token, remember)
	return se, nil
}

type stateResponse struct {
	SetupRequired bool   `json:"setupRequired"`
	SetupAllowed  bool   `json:"setupAllowed"`
	SetupReason   string `json:"setupReason,omitempty"` // expired | not_local
	SetupToken    bool   `json:"setupTokenRequired"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	CSRF          string `json:"csrf,omitempty"`
	Version       string `json:"version"`
	Language      string `json:"language"`
	HTTPS         bool   `json:"https"`
}

// setupCheck reports whether the initial account may be created from r.
func (s *Server) setupCheck(r *http.Request) (bool, string) {
	if s.now().Sub(s.d.StartedAt) > SetupWindow {
		return false, "expired"
	}
	ip := remoteIP(r) // never X-Forwarded-For here
	if ip.IsLoopback() {
		return true, ""
	}
	if s.d.LocalPrefixes != nil {
		for _, p := range s.d.LocalPrefixes() {
			if p.Contains(ip) {
				return true, ""
			}
		}
	}
	return false, "not_local"
}

func (s *Server) handleAuthState(w http.ResponseWriter, r *http.Request) {
	n, err := s.d.Store.UserCount(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st := s.d.Settings()
	resp := stateResponse{SetupRequired: n == 0, Version: s.d.Version, Language: st.Language, HTTPS: s.isHTTPS(r),
		SetupToken: s.d.Env.SetupToken != ""}
	if n == 0 {
		resp.SetupAllowed, resp.SetupReason = s.setupCheck(r)
	} else if si := s.lookupSession(r); si != nil {
		resp.Authenticated, resp.Username, resp.CSRF = true, si.user.Username, si.session.CSRFToken
	}
	writeJSON(w, http.StatusOK, resp)
}

type setupRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Language    string `json:"language"`
	Timezone    string `json:"timezone"`
	NetworkName string `json:"networkName"`
	Token       string `json:"token"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if !readJSON(w, r, &req) {
		return
	}
	key := "setup/" + remoteIP(r).String()
	if ok, wait := s.limiter.Allow(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	if ok, reason := s.setupCheck(r); !ok {
		writeError(w, http.StatusForbidden, "setup_"+reason, "")
		return
	}
	if s.d.Env.SetupToken != "" && subtle.ConstantTimeCompare([]byte(req.Token), []byte(s.d.Env.SetupToken)) != 1 {
		s.limiter.Fail(key)
		writeError(w, http.StatusForbidden, "setup_token", "")
		return
	}
	if !auth.ValidUsername(req.Username) {
		writeError(w, http.StatusBadRequest, "invalid_username", "")
		return
	}
	if err := auth.CheckPassword(req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "")
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	hash, err := auth.HashPassword(r.Context(), req.Password)
	if errors.Is(err, auth.ErrBusy) {
		writeError(w, http.StatusServiceUnavailable, "busy", "")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.d.Store.CreateFirstUser(r.Context(), req.Username, hash)
	if errors.Is(err, store.ErrExists) {
		writeError(w, http.StatusConflict, "already_setup", "")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st := s.d.Settings()
	if req.Language == "de" || req.Language == "en" {
		st.Language = req.Language
	}
	if _, err := time.LoadLocation(req.Timezone); err == nil && req.Timezone != "" && len(req.Timezone) <= 64 {
		st.Timezone = req.Timezone
	}
	if name := textutil.Clean(req.NetworkName, 64); name != "" {
		st.NetworkName = name
	}
	if err := s.d.ApplySettings(r.Context(), st); err != nil {
		s.d.Log.Warn("could not store initial settings", "err", err)
	}
	se, err := s.createSession(r.Context(), w, r, u, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.d.Monitor.AddEvent(r.Context(), "auth_setup", "info", 0, map[string]any{"user": u.Username, "ip": remoteIP(r).String()})
	s.d.Log.Info("initial account created", "user", u.Username)
	writeJSON(w, http.StatusOK, stateResponse{Authenticated: true, Username: u.Username, CSRF: se.CSRFToken,
		Version: s.d.Version, Language: st.Language, HTTPS: s.isHTTPS(r)})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !readJSON(w, r, &req) {
		return
	}
	ip := s.clientIP(r)
	key := "login/" + ip.String()
	if ok, wait := s.limiter.Allow(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	if len(req.Username) > 64 || len(req.Password) > 512 {
		writeError(w, http.StatusBadRequest, "invalid_credentials", "")
		return
	}
	u, err := s.d.Store.GetUserByName(r.Context(), req.Username)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	ok, verr := auth.VerifyPassword(r.Context(), u.PasswordHash, req.Password) // dummy hash if user unknown
	if errors.Is(verr, auth.ErrBusy) {
		w.Header().Set("Retry-After", "3")
		writeError(w, http.StatusServiceUnavailable, "busy", "")
		return
	}
	if !ok || u.ID == 0 {
		s.limiter.Fail(key)
		s.d.Monitor.AddEvent(r.Context(), "auth_failed", "warning", 0, map[string]any{"ip": ip.String(),
			"user": textutil.Clean(req.Username, 64)})
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "")
		return
	}
	s.limiter.Succeed(key)
	se, err := s.createSession(r.Context(), w, r, u, req.Remember)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.d.Monitor.AddEvent(r.Context(), "auth_login", "info", 0, map[string]any{"ip": ip.String(), "user": u.Username})
	st := s.d.Settings()
	writeJSON(w, http.StatusOK, stateResponse{Authenticated: true, Username: u.Username, CSRF: se.CSRFToken,
		Version: s.d.Version, Language: st.Language, HTTPS: s.isHTTPS(r)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, si *sessionInfo) {
	_ = s.d.Store.DeleteSession(r.Context(), si.user.ID, si.session.ID)
	s.streams.closeSession(si.session.ID)
	s.clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

type passwordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request, si *sessionInfo) {
	var req passwordRequest
	if !readJSON(w, r, &req) {
		return
	}
	key := "password/" + strconv.FormatInt(si.user.ID, 10)
	if ok, wait := s.limiter.Allow(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	ok, err := auth.VerifyPassword(r.Context(), si.user.PasswordHash, req.Current)
	if err != nil || !ok {
		s.limiter.Fail(key)
		writeError(w, http.StatusForbidden, "wrong_password", "")
		return
	}
	if err := auth.CheckPassword(si.user.Username, req.New); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "")
		return
	}
	hash, err := auth.HashPassword(r.Context(), req.New)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.d.Store.SetPassword(r.Context(), si.user.ID, hash, si.session.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.limiter.Succeed(key)
	s.streams.closeAllExcept(si.session.ID)
	s.d.Monitor.AddEvent(r.Context(), "auth_password", "info", 0, map[string]any{"ip": s.clientIP(r).String()})
	w.WriteHeader(http.StatusNoContent)
}

type sessionView struct {
	store.Session
	Current bool `json:"current"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request, si *sessionInfo) {
	list, err := s.d.Store.ListSessions(r.Context(), si.user.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]sessionView, 0, len(list))
	for _, se := range list {
		out = append(out, sessionView{Session: se, Current: se.ID == si.session.ID})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request, si *sessionInfo) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "")
		return
	}
	if err := s.d.Store.DeleteSession(r.Context(), si.user.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.streams.closeSession(id)
	if id == si.session.ID {
		s.clearCookie(w, r)
	}
	w.WriteHeader(http.StatusNoContent)
}

// LocalPrefixes returns the networks directly attached to the host
// (including container bridges, so a reverse proxy container counts as local).
func LocalPrefixes() []netip.Prefix {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if p, err := netip.ParsePrefix(ipn.String()); err == nil {
				out = append(out, p.Masked())
			}
		}
	}
	return out
}
