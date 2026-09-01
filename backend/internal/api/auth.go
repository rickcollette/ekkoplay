package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	authn "ekkoplayer/internal/auth"
)

type adminContextKey struct{}
type loginAttempt struct {
	start time.Time
	count int
}

func adminFromContext(ctx context.Context) (authn.Admin, bool) {
	a, ok := ctx.Value(adminContextKey{}).(authn.Admin)
	return a, ok
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r, ok := s.authenticateAdmin(w, r)
		if !ok {
			return
		}
		next(w, r)
	}
}

func (s *Server) authenticateAdmin(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if s.auth == nil {
		writeErr(w, 503, errors.New("administrator authentication is not configured"))
		return r, false
	}
	c, err := r.Cookie(authn.AccessCookie)
	if err != nil {
		writeErr(w, 401, authn.ErrInvalidToken)
		return r, false
	}
	a, err := s.auth.Access(c.Value)
	if err != nil {
		writeErr(w, 401, err)
		return r, false
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && !sameOrigin(r) {
		writeErr(w, 403, errors.New("cross-origin request denied"))
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), adminContextKey{}, a)), true
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (s *Server) setTokens(w http.ResponseWriter, t authn.Tokens) {
	http.SetCookie(w, &http.Cookie{Name: authn.AccessCookie, Value: t.Access, Path: "/", Expires: t.AccessExpires, MaxAge: int(time.Until(t.AccessExpires).Seconds()), HttpOnly: true, Secure: s.auth.SecureCookies(), SameSite: http.SameSiteStrictMode})
	http.SetCookie(w, &http.Cookie{Name: authn.RefreshCookie, Value: t.Refresh, Path: "/api/v1/auth", Expires: t.RefreshExpiry, MaxAge: int(time.Until(t.RefreshExpiry).Seconds()), HttpOnly: true, Secure: s.auth.SecureCookies(), SameSite: http.SameSiteStrictMode})
}
func (s *Server) clearTokens(w http.ResponseWriter) {
	for _, c := range []struct{ name, path string }{{authn.AccessCookie, "/"}, {authn.RefreshCookie, "/api/v1/auth"}} {
		http.SetCookie(w, &http.Cookie{Name: c.name, Value: "", Path: c.path, MaxAge: -1, HttpOnly: true, Secure: s.auth != nil && s.auth.SecureCookies(), SameSite: http.SameSiteStrictMode})
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeErr(w, 503, errors.New("administrator authentication is not configured"))
		return
	}
	if !sameOrigin(r) {
		writeErr(w, 403, errors.New("cross-origin request denied"))
		return
	}
	var b struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	key := clientIP(r) + "|" + authn.NormalizeUsername(b.Username)
	if !s.allowLogin(key) {
		writeErr(w, http.StatusTooManyRequests, errors.New("too many login attempts; try again shortly"))
		return
	}
	t, err := s.auth.Login(r.Context(), b.Username, b.Password)
	if err != nil {
		writeErr(w, 401, authn.ErrInvalidCredentials)
		return
	}
	s.clearLogin(key)
	s.setTokens(w, t)
	writeJSON(w, 200, map[string]any{"admin": t.Admin, "access_expires_at": t.AccessExpires, "refresh_expires_at": t.RefreshExpiry})
}
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, 403, errors.New("cross-origin request denied"))
		return
	}
	c, err := r.Cookie(authn.RefreshCookie)
	if err != nil {
		writeErr(w, 401, authn.ErrInvalidToken)
		return
	}
	t, err := s.auth.Refresh(r.Context(), c.Value)
	if err != nil {
		s.clearTokens(w)
		writeErr(w, 401, err)
		return
	}
	s.setTokens(w, t)
	writeJSON(w, 200, map[string]any{"admin": t.Admin, "access_expires_at": t.AccessExpires, "refresh_expires_at": t.RefreshExpiry})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	a, _ := adminFromContext(r.Context())
	writeJSON(w, 200, map[string]any{"admin": a})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie(authn.RefreshCookie); e == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	s.clearTokens(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) logoutAll(w http.ResponseWriter, r *http.Request) {
	a, _ := adminFromContext(r.Context())
	if err := s.auth.LogoutAll(r.Context(), a.ID); err != nil {
		writeErr(w, 500, err)
		return
	}
	s.clearTokens(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	a, _ := adminFromContext(r.Context())
	var b struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.auth.ChangePassword(r.Context(), a.ID, b.Current, b.Next); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.clearTokens(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func clientIP(r *http.Request) string {
	h, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			if forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); forwarded != nil {
				return forwarded.String()
			}
		}
		return h
	}
	return r.RemoteAddr
}
func (s *Server) allowLogin(key string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	x := s.loginAttempts[key]
	if now.Sub(x.start) > time.Minute {
		x = loginAttempt{start: now}
	}
	x.count++
	s.loginAttempts[key] = x
	return x.count <= 5
}
func (s *Server) clearLogin(key string) {
	s.loginMu.Lock()
	delete(s.loginAttempts, key)
	s.loginMu.Unlock()
}
