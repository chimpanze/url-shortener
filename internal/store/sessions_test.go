package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateAndGetSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	sess := Session{
		Token: "tok-1", CSRFToken: "csrf-1",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession(ctx, "tok-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CSRFToken != "csrf-1" {
		t.Errorf("csrf = %q", got.CSRFToken)
	}
}

func TestGetSessionExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	_ = s.CreateSession(ctx, Session{
		Token: "tok", CSRFToken: "c",
		CreatedAt: past.Add(-time.Hour), ExpiresAt: past,
	})
	_, err := s.GetSession(ctx, "tok")
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	_ = s.CreateSession(ctx, Session{Token: "tok", CSRFToken: "c", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err := s.DeleteSession(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetSession(ctx, "tok")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
