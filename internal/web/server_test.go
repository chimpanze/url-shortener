package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterServesStaticCSS(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/static/style.css", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if rw.Body.Len() == 0 {
		t.Fatal("empty body")
	}
}

func TestUnknownSlugReturns404(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Errorf("status = %d", rw.Code)
	}
}
