package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRedirectHit(t *testing.T) {
	env := newTestEnv(t)
	id, err := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = id

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req.Header.Set("Referer", "https://r.example")
	req.Header.Set("User-Agent", "test-ua")
	rw := httptest.NewRecorder()

	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rw.Code)
	}
	if loc := rw.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("location = %q", loc)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		n, _ := env.store.CountClicks(context.Background(), id)
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected 1 click, got %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRedirectMiss(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Errorf("status = %d", rw.Code)
	}
}
