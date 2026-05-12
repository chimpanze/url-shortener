package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/store"
)

func TestCSRFMiddlewareRejectsMissing(t *testing.T) {
	env := newTestEnv(t)
	hash, _ := auth.HashPassword("pw")
	_ = env.store.SetAdminPasswordHash(context.Background(), hash)

	sess := store.Session{
		Token: "tok-1", CSRFToken: "csrf-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = env.store.CreateSession(context.Background(), sess)

	handler := env.server.deps.Sessions.RequireAuth(env.server.csrfProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/admin/anything", nil)
	req.AddCookie(&http.Cookie{Name: "ffsbz_session", Value: "tok-1"})
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rw.Code)
	}
}

func TestCSRFMiddlewareAcceptsMatching(t *testing.T) {
	env := newTestEnv(t)
	hash, _ := auth.HashPassword("pw")
	_ = env.store.SetAdminPasswordHash(context.Background(), hash)

	sess := store.Session{
		Token: "tok-2", CSRFToken: "csrf-2",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = env.store.CreateSession(context.Background(), sess)

	handler := env.server.deps.Sessions.RequireAuth(env.server.csrfProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	form := url.Values{"csrf_token": []string{"csrf-2"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/anything", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ffsbz_session", Value: "tok-2"})
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rw.Code)
	}
}
