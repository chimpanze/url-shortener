package store

import (
	"context"
	"errors"
	"testing"
)

func TestSetAndGetAdminPasswordHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.SetAdminPasswordHash(ctx, "hash1"); err != nil {
		t.Fatal(err)
	}
	h, err := s.GetAdminPasswordHash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if h != "hash1" {
		t.Errorf("hash = %q", h)
	}

	if err := s.SetAdminPasswordHash(ctx, "hash2"); err != nil {
		t.Fatal(err)
	}
	h, _ = s.GetAdminPasswordHash(ctx)
	if h != "hash2" {
		t.Errorf("hash after update = %q", h)
	}
}

func TestGetAdminPasswordHashMissing(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetAdminPasswordHash(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
