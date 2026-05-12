package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"url-shortener/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLoginCreatesSession(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	hash, _ := HashPassword("pw")
	_ = s.SetAdminPasswordHash(ctx, hash)

	mgr := NewSessionManager(s, SessionConfig{TTL: time.Hour, CookieName: "urlshortener_session"})
	w := httptest.NewRecorder()
	if err := mgr.Login(ctx, w, "pw"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 || cookies[0].Name != "urlshortener_session" {
		t.Fatal("expected session cookie")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s := newStore(t)
	hash, _ := HashPassword("pw")
	_ = s.SetAdminPasswordHash(context.Background(), hash)
	mgr := NewSessionManager(s, SessionConfig{TTL: time.Hour, CookieName: "urlshortener_session"})
	w := httptest.NewRecorder()
	if err := mgr.Login(context.Background(), w, "wrong"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRequireAuthMiddleware(t *testing.T) {
	s := newStore(t)
	hash, _ := HashPassword("pw")
	_ = s.SetAdminPasswordHash(context.Background(), hash)
	mgr := NewSessionManager(s, SessionConfig{TTL: time.Hour, CookieName: "urlshortener_session", LoginPath: "/admin/login"})

	// Authenticated.
	w := httptest.NewRecorder()
	_ = mgr.Login(context.Background(), w, "pw")
	cookie := w.Result().Cookies()[0]

	called := false
	handler := mgr.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if !called {
		t.Error("expected handler called")
	}

	// Unauthenticated.
	req2 := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rw2 := httptest.NewRecorder()
	handler.ServeHTTP(rw2, req2)
	if rw2.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rw2.Code)
	}
}
