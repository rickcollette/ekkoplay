package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ekkoplayer/internal/auth"
	"ekkoplayer/internal/db"
)

func authServer(t *testing.T) *Server {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	a, err := auth.New(store.DB, []byte("0123456789012345678901234567890123456789012345678901234567890123"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.CreateInitialAdmin(context.Background(), "admin", "test administrator password"); err != nil {
		t.Fatal(err)
	}
	return &Server{auth: a, loginAttempts: make(map[string]loginAttempt)}
}
func TestLoginCookiesAndAdminBoundary(t *testing.T) {
	s := authServer(t)
	login := httptest.NewRequest("POST", "http://player.local/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"test administrator password"}`))
	login.Host = "player.local"
	login.Header.Set("Origin", "http://player.local")
	w := httptest.NewRecorder()
	s.login(w, login)
	if w.Code != 200 {
		t.Fatalf("login status %d: %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("got %d cookies", len(cookies))
	}
	for _, c := range cookies {
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Secure {
			t.Fatalf("unsafe HTTP cookie: %+v", c)
		}
	}
	protected := s.requireAdmin(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	r := httptest.NewRequest("POST", "http://player.local/api/v1/admin/test", nil)
	r.Host = "player.local"
	r.Header.Set("Origin", "http://evil.local")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	denied := httptest.NewRecorder()
	protected(denied, r)
	if denied.Code != 403 {
		t.Fatalf("cross-origin status %d", denied.Code)
	}
	r.Header.Set("Origin", "http://player.local")
	allowed := httptest.NewRecorder()
	protected(allowed, r)
	if allowed.Code != 204 {
		t.Fatalf("same-origin status %d: %s", allowed.Code, allowed.Body.String())
	}
}

func TestAdminBoundaryRejectsMissingAccessCookie(t *testing.T) {
	s := authServer(t)
	w := httptest.NewRecorder()
	s.requireAdmin(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler called") })(w, httptest.NewRequest("GET", "http://player.local/api/v1/admin/stats", nil))
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
}
