package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"url-shortener/internal/store"
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

func TestAdminDetailShowsClicks(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	_ = env.store.InsertClicks(context.Background(), []store.ClickRecord{
		{LinkID: id, TS: time.Now(), Referer: "https://r.example", UserAgent: "ua-1"},
	})
	cookie := authCookie(t, env)

	req := httptest.NewRequest(http.MethodGet, "/admin/links/"+strconv.FormatInt(id, 10), nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "ua-1") || !strings.Contains(body, "r.example") {
		t.Errorf("missing click data: %s", body)
	}
}

func TestAdminDelete(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{"csrf_token": []string{sess.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+strconv.FormatInt(id, 10)+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	if _, err := env.store.GetLinkByID(context.Background(), id); err != store.ErrNotFound {
		t.Errorf("link not deleted (err=%v)", err)
	}
}

func TestAdminEditPage(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://old.example", time.Now())
	cookie := authCookie(t, env)
	req := httptest.NewRequest(http.MethodGet, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "https://old.example") {
		t.Errorf("expected old destination prefilled")
	}
}

func TestAdminEditPost(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://old.example", time.Now())
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"destination": []string{"https://new.example"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	link, _ := env.store.GetLinkByID(context.Background(), id)
	if link.Destination != "https://new.example" {
		t.Errorf("destination = %q", link.Destination)
	}
}

func TestAdminEditRejectsBadURL(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"destination": []string{"javascript:alert(1)"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rw.Code)
	}
}
