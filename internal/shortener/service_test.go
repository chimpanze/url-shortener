package shortener

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ffs.bz/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s), s
}

func TestCreateLinkRandom(t *testing.T) {
	svc, _ := newTestService(t)
	link, err := svc.CreateLink(context.Background(), "", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if link.Slug == "" {
		t.Error("expected non-empty slug")
	}
}

func TestCreateLinkCustomSlug(t *testing.T) {
	svc, _ := newTestService(t)
	link, err := svc.CreateLink(context.Background(), "blog", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if link.Slug != "blog" {
		t.Errorf("slug = %q", link.Slug)
	}
}

func TestCreateLinkRejectsBadSlug(t *testing.T) {
	svc, _ := newTestService(t)
	cases := []string{"with space", "with/slash", "ümlaut", "admin", "static", "health", ""}
	// Note: "" is OK (means random) — remove from list.
	cases = cases[:len(cases)-1]
	for _, c := range cases {
		_, err := svc.CreateLink(context.Background(), c, "https://example.com")
		if !errors.Is(err, ErrInvalidSlug) && !errors.Is(err, ErrReservedSlug) {
			t.Errorf("slug %q: expected invalid/reserved, got %v", c, err)
		}
	}
}

func TestCreateLinkRejectsBadURL(t *testing.T) {
	svc, _ := newTestService(t)
	cases := []string{"not a url", "ftp://example.com", "javascript:alert(1)", "/relative"}
	for _, c := range cases {
		_, err := svc.CreateLink(context.Background(), "", c)
		if !errors.Is(err, ErrInvalidURL) {
			t.Errorf("url %q: expected ErrInvalidURL, got %v", c, err)
		}
	}
}

func TestCreateLinkCustomSlugCollision(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.CreateLink(context.Background(), "x", "https://example.com"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateLink(context.Background(), "x", "https://other.com")
	if !errors.Is(err, ErrSlugTaken) {
		t.Errorf("expected ErrSlugTaken, got %v", err)
	}
}

func TestCreateLinkRandomExhausted(t *testing.T) {
	svc, _ := newTestService(t)
	// Force every random attempt to return "stuck", and pre-occupy that slug.
	svc.genCode = func(int) (string, error) { return "stuck", nil }
	if _, err := svc.CreateLink(context.Background(), "stuck", "https://example.com"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateLink(context.Background(), "", "https://other.example")
	if !errors.Is(err, ErrSlugExhausted) {
		t.Errorf("expected ErrSlugExhausted, got %v", err)
	}
}
