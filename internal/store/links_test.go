package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateLinkAndGetBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateLink(ctx, "abc", "https://example.com", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	link, err := s.GetLinkBySlug(ctx, "abc")
	if err != nil {
		t.Fatalf("GetLinkBySlug: %v", err)
	}
	if link.Destination != "https://example.com" {
		t.Errorf("destination = %q", link.Destination)
	}
	if link.Slug != "abc" {
		t.Errorf("slug = %q", link.Slug)
	}
}

func TestCreateLinkDuplicateSlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := s.CreateLink(ctx, "abc", "https://example.com", now); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateLink(ctx, "abc", "https://other.com", now)
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestGetLinkBySlugNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetLinkBySlug(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListLinksWithCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	id1, _ := s.CreateLink(ctx, "a", "https://a.example", now)
	id2, _ := s.CreateLink(ctx, "b", "https://b.example", now.Add(time.Second))

	// Insert clicks directly (Clicks task adds the API later; raw insert here)
	for i := 0; i < 3; i++ {
		_, err := s.DB().ExecContext(ctx,
			`INSERT INTO clicks (link_id, ts) VALUES (?, ?)`, id1, now.Unix())
		if err != nil {
			t.Fatal(err)
		}
	}

	list, err := s.ListLinksWithCounts(ctx)
	if err != nil {
		t.Fatalf("ListLinksWithCounts: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 links, got %d", len(list))
	}
	// Newest first.
	if list[0].ID != id2 {
		t.Errorf("expected newest first; got id=%d", list[0].ID)
	}
	var aRow *LinkWithCount
	for i := range list {
		if list[i].ID == id1 {
			aRow = &list[i]
		}
	}
	if aRow == nil || aRow.Clicks != 3 {
		t.Errorf("expected 3 clicks on link a, got %+v", aRow)
	}
}

func TestUpdateDestination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.CreateLink(ctx, "x", "https://old.example", time.Now())
	if err := s.UpdateDestination(ctx, id, "https://new.example"); err != nil {
		t.Fatal(err)
	}
	link, _ := s.GetLinkByID(ctx, id)
	if link.Destination != "https://new.example" {
		t.Errorf("destination = %q", link.Destination)
	}
}

func TestDeleteLinkCascadesClicks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.CreateLink(ctx, "x", "https://x.example", time.Now())
	_, err := s.DB().ExecContext(ctx,
		`INSERT INTO clicks (link_id, ts) VALUES (?, ?)`, id, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM clicks WHERE link_id = ?`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("clicks not cascaded, got %d remaining", n)
	}
}
