package clicklog

import (
	"context"
	"path/filepath"
	"sync/atomic"
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

func TestLoggerWritesEvents(t *testing.T) {
	s := newStore(t)
	id, _ := s.CreateLink(context.Background(), "x", "https://x.example", time.Now())

	lg := New(s, Config{BufferSize: 16, FlushMaxBatch: 4, FlushInterval: 20 * time.Millisecond})
	lg.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		lg.Shutdown(ctx)
	}()

	for i := 0; i < 10; i++ {
		lg.Enqueue(Event{LinkID: id, TS: time.Unix(int64(i), 0), Referer: "r", UserAgent: "ua"})
	}

	// Wait until all 10 are written, with a timeout.
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, _ := s.CountClicks(context.Background(), id)
		if n == 10 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected 10 clicks, got %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoggerOverflowDropsEvents(t *testing.T) {
	s := newStore(t)
	id, _ := s.CreateLink(context.Background(), "x", "https://x.example", time.Now())

	// Tiny buffer + no worker started, so every enqueue past 2 must be dropped.
	lg := New(s, Config{BufferSize: 2, FlushMaxBatch: 1, FlushInterval: time.Hour})
	// Intentionally do NOT Start() — fill the channel.

	dropped := int64(0)
	for i := 0; i < 10; i++ {
		if !lg.tryEnqueue(Event{LinkID: id, TS: time.Now()}) {
			atomic.AddInt64(&dropped, 1)
		}
	}
	if dropped == 0 {
		t.Fatal("expected some drops, got none")
	}
}
