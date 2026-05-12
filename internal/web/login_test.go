package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ffs.bz/internal/auth"
)

func setAdminPassword(t *testing.T, env *testEnv, pw string) {
	t.Helper()
	hash, _ := auth.HashPassword(pw)
	if err := env.store.SetAdminPasswordHash(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
}

func authCookie(t *testing.T, env *testEnv) *http.Cookie {
	t.Helper()
	setAdminPassword(t, env, "pw")
	form := url.Values{"password": []string{"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Fatalf("authCookie: login status = %d", rw.Code)
	}
	cookies := rw.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("authCookie: no cookie returned")
	}
	return cookies[0]
}

func TestLoginPageRenders(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "Admin login") {
		t.Errorf("expected login page content")
	}
}

func TestLoginSuccess(t *testing.T) {
	env := newTestEnv(t)
	setAdminPassword(t, env, "pw")

	form := url.Values{"password": []string{"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	cookies := rw.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "ffsbz_session" {
		t.Fatalf("expected session cookie, got %+v", cookies)
	}
}

func TestLoginWrong(t *testing.T) {
	env := newTestEnv(t)
	setAdminPassword(t, env, "pw")

	form := url.Values{"password": []string{"nope"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rw.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	logoutForm := url.Values{"csrf_token": []string{sess.CSRFToken}}
	logoutReq := httptest.NewRequest(http.MethodPost, "/admin/logout", strings.NewReader(logoutForm.Encode()))
	logoutReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	logoutReq.AddCookie(cookie)
	logoutRW := httptest.NewRecorder()
	env.server.Router().ServeHTTP(logoutRW, logoutReq)
	if logoutRW.Code != http.StatusSeeOther {
		t.Errorf("logout status = %d", logoutRW.Code)
	}

	gone := logoutRW.Result().Cookies()
	if len(gone) == 0 || gone[0].MaxAge >= 0 {
		t.Errorf("expected cookie cleared, got %+v", gone)
	}
}
