package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAdminNewPage(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	req := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "New link") {
		t.Errorf("missing form")
	}
}

func TestAdminNewCreatesLinkWithCustomSlug(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"slug":        []string{"blog"},
		"destination": []string{"https://example.com"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	link, err := env.store.GetLinkBySlug(context.Background(), "blog")
	if err != nil {
		t.Fatalf("link not created: %v", err)
	}
	if link.Destination != "https://example.com" {
		t.Errorf("destination = %q", link.Destination)
	}
}

func TestAdminNewValidationError(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"destination": []string{"not-a-url"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rw.Code)
	}
}
