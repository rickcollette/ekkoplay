package auth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"ekkoplayer/internal/db"
)

func testService(t *testing.T) (*Service, *db.Store) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s, err := New(store.DB, []byte("0123456789012345678901234567890123456789012345678901234567890123"), false)
	if err != nil {
		t.Fatal(err)
	}
	return s, store
}

func TestPasswordAndRotatingRefreshLifecycle(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	if err := s.CreateInitialAdmin(ctx, "Admin.User", "this is a strong test password"); err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := s.db.QueryRow(`SELECT password_hash FROM admins`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") || strings.Contains(encoded, "strong test password") {
		t.Fatalf("password was not stored as Argon2id: %q", encoded)
	}
	if _, err := s.Login(ctx, "admin.user", "wrong password"); err != ErrInvalidCredentials {
		t.Fatalf("wrong-password error=%v", err)
	}
	first, err := s.Login(ctx, "ADMIN.USER", "this is a strong test password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Access(first.Access); err != nil {
		t.Fatal(err)
	}
	second, err := s.Refresh(ctx, first.Refresh)
	if err != nil {
		t.Fatal(err)
	}
	if second.Refresh == first.Refresh {
		t.Fatal("refresh token was not rotated")
	}
	if _, err = s.Refresh(ctx, first.Refresh); err != ErrRefreshReuse {
		t.Fatalf("replay error=%v", err)
	}
	if _, err = s.Refresh(ctx, second.Refresh); err != ErrRefreshReuse {
		t.Fatalf("family was not revoked after replay: %v", err)
	}
}

func TestPasswordResetRevokesRefreshSessions(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	if err := s.CreateInitialAdmin(ctx, "admin", "original password value"); err != nil {
		t.Fatal(err)
	}
	tok, err := s.Login(ctx, "admin", "original password value")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, "admin", "replacement password value"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Refresh(ctx, tok.Refresh); err != ErrRefreshReuse {
		t.Fatalf("refresh survived reset: %v", err)
	}
	if _, err = s.Login(ctx, "admin", "replacement password value"); err != nil {
		t.Fatal(err)
	}
}
