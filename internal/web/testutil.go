package web

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"url-shortener/internal/auth"
	"url-shortener/internal/clicklog"
	"url-shortener/internal/shortener"
	"url-shortener/internal/store"
)

type testEnv struct {
	store    *store.Store
	server   *Server
	sessions *auth.SessionManager
	clicks   *clicklog.Logger
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	mgr := auth.NewSessionManager(s, auth.SessionConfig{
		CookieName: "urlshortener_session", TTL: time.Hour, LoginPath: "/admin/login",
	})
	cl := clicklog.New(s, clicklog.Config{
		BufferSize: 16, FlushMaxBatch: 4, FlushInterval: 20 * time.Millisecond,
	})
	cl.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cl.Shutdown(ctx)
	})

	svr := NewServer(Deps{
		Store:     s,
		Shortener: shortener.New(s),
		Sessions:  mgr,
		Clicks:    cl,
	})
	return &testEnv{store: s, server: svr, sessions: mgr, clicks: cl}
}
