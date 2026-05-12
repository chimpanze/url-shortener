package clicklog

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"url-shortener/internal/store"
)

type Event struct {
	LinkID    int64
	TS        time.Time
	Referer   string
	UserAgent string
}

type Config struct {
	BufferSize    int
	FlushMaxBatch int
	FlushInterval time.Duration
}

func DefaultConfig() Config {
	return Config{BufferSize: 1024, FlushMaxBatch: 64, FlushInterval: 200 * time.Millisecond}
}

type Logger struct {
	store    *store.Store
	cfg      Config
	ch       chan Event
	done     chan struct{}
	dropped  int64
	stopOnce sync.Once
}

func New(s *store.Store, cfg Config) *Logger {
	if cfg.BufferSize <= 0 {
		cfg = DefaultConfig()
	}
	return &Logger{
		store: s,
		cfg:   cfg,
		ch:    make(chan Event, cfg.BufferSize),
		done:  make(chan struct{}),
	}
}

func (l *Logger) Start() {
	go l.run()
}

func (l *Logger) Enqueue(e Event) {
	if !l.tryEnqueue(e) {
		atomic.AddInt64(&l.dropped, 1)
	}
}

func (l *Logger) tryEnqueue(e Event) bool {
	select {
	case l.ch <- e:
		return true
	default:
		return false
	}
}

func (l *Logger) Shutdown(ctx context.Context) {
	l.stopOnce.Do(func() { close(l.ch) })
	select {
	case <-l.done:
	case <-ctx.Done():
	}
}

func (l *Logger) run() {
	defer close(l.done)
	ticker := time.NewTicker(l.cfg.FlushInterval)
	defer ticker.Stop()
	dropReport := time.NewTicker(time.Minute)
	defer dropReport.Stop()

	var batch []Event
	flush := func() {
		if len(batch) == 0 {
			return
		}
		recs := make([]store.ClickRecord, len(batch))
		for i, e := range batch {
			recs[i] = store.ClickRecord{
				LinkID: e.LinkID, TS: e.TS, Referer: e.Referer, UserAgent: e.UserAgent,
			}
		}
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := l.store.InsertClicks(ctx, recs); err != nil {
				slog.Error("clicklog flush failed", "err", err, "n", len(batch))
			}
		}()
		batch = batch[:0]
	}

	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= l.cfg.FlushMaxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-dropReport.C:
			if d := atomic.SwapInt64(&l.dropped, 0); d > 0 {
				slog.Warn("clicklog dropped events", "n", d)
			}
		}
	}
}
