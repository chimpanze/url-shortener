package store

import (
	"context"
	"testing"
	"time"
)

func TestInsertClicksBatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.CreateLink(ctx, "x", "https://x.example", time.Now())

	evs := []ClickRecord{
		{LinkID: id, TS: time.Unix(1, 0), Referer: "https://r1", UserAgent: "ua1"},
		{LinkID: id, TS: time.Unix(2, 0), Referer: "", UserAgent: "ua2"},
		{LinkID: id, TS: time.Unix(3, 0), Referer: "https://r3", UserAgent: ""},
	}
	if err := s.InsertClicks(ctx, evs); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListClicks(ctx, id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// newest first
	if got[0].TS.Unix() != 3 {
		t.Errorf("expected newest TS=3, got %d", got[0].TS.Unix())
	}
}

func TestInsertClicksEmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertClicks(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestListClicksRespectsLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.CreateLink(ctx, "x", "https://x.example", time.Now())

	var evs []ClickRecord
	for i := 1; i <= 5; i++ {
		evs = append(evs, ClickRecord{LinkID: id, TS: time.Unix(int64(i), 0)})
	}
	if err := s.InsertClicks(ctx, evs); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListClicks(ctx, id, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}
