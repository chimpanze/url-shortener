package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/shortener"
	"ffs.bz/internal/store"
	"ffs.bz/internal/web"
)

func TestEndToEndCreateAndRedirect(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	hash, _ := auth.HashPassword("hunter2x9")
	_ = s.SetAdminPasswordHash(context.Background(), hash)

	mgr := auth.NewSessionManager(s, auth.SessionConfig{
		CookieName: "ffsbz_session", TTL: time.Hour, LoginPath: "/admin/login",
	})
	cl := clicklog.New(s, clicklog.Config{BufferSize: 16, FlushMaxBatch: 4, FlushInterval: 20 * time.Millisecond})
	cl.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cl.Shutdown(ctx)
	}()

	svr := web.NewServer(web.Deps{
		Store:     s,
		Shortener: shortener.New(s),
		Sessions:  mgr,
		Clicks:    cl,
	})

	// 1. Log in.
	form := url.Values{"password": []string{"hunter2x9"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	cookie := rw.Result().Cookies()[0]

	// 2. Create a link.
	sess, _ := s.GetSession(context.Background(), cookie.Value)
	form = url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"slug":        []string{"blog"},
		"destination": []string{"https://example.com"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw = httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Fatalf("create status %d", rw.Code)
	}

	// 3. Visit /blog -> 302 to example.com.
	req = httptest.NewRequest(http.MethodGet, "/blog", nil)
	req.Header.Set("Referer", "https://hn.example")
	req.Header.Set("User-Agent", "e2e-bot")
	rw = httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusFound {
		t.Errorf("redirect status %d", rw.Code)
	}
	if rw.Header().Get("Location") != "https://example.com" {
		t.Errorf("location %q", rw.Header().Get("Location"))
	}

	// 4. Wait for the click to land.
	link, _ := s.GetLinkBySlug(context.Background(), "blog")
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, _ := s.CountClicks(context.Background(), link.ID)
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("click not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 5. Detail page shows the click.
	req = httptest.NewRequest(http.MethodGet, "/admin/links/1", nil)
	req.AddCookie(cookie)
	rw = httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	if !strings.Contains(rw.Body.String(), "e2e-bot") {
		t.Errorf("user-agent missing from detail page")
	}
}
