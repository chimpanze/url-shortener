package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminListShowsLinks(t *testing.T) {
	env := newTestEnv(t)
	_, _ = env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "abc") || !strings.Contains(body, "https://example.com") {
		t.Errorf("missing link in body: %s", body)
	}
}

func TestAdminListRedirectsWithoutSession(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Errorf("status = %d", rw.Code)
	}
}
