package shortener

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"ffs.bz/internal/store"
)

var (
	ErrInvalidSlug  = errors.New("shortener: invalid slug")
	ErrReservedSlug = errors.New("shortener: slug is reserved")
	ErrInvalidURL   = errors.New("shortener: invalid destination URL")
	ErrSlugTaken    = errors.New("shortener: slug already taken")
	ErrSlugExhausted = errors.New("shortener: couldn't allocate a random slug")
)

var (
	slugRegex     = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	reservedSlugs = map[string]bool{"admin": true, "static": true, "health": true}
)

const (
	randomCodeLength = 6
	maxCodeRetries   = 5
)

type Service struct {
	store *store.Store
	now   func() time.Time
}

func New(s *store.Store) *Service {
	return &Service{store: s, now: time.Now}
}

func (s *Service) CreateLink(ctx context.Context, slug, destination string) (*store.Link, error) {
	if err := validateDestination(destination); err != nil {
		return nil, err
	}
	if slug == "" {
		return s.createRandom(ctx, destination)
	}
	if !slugRegex.MatchString(slug) {
		return nil, ErrInvalidSlug
	}
	if reservedSlugs[slug] {
		return nil, ErrReservedSlug
	}
	id, err := s.store.CreateLink(ctx, slug, destination, s.now())
	if err != nil {
		if errors.Is(err, store.ErrSlugTaken) {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	return s.store.GetLinkByID(ctx, id)
}

func (s *Service) createRandom(ctx context.Context, destination string) (*store.Link, error) {
	for i := 0; i < maxCodeRetries; i++ {
		code, err := RandomCode(randomCodeLength)
		if err != nil {
			return nil, err
		}
		id, err := s.store.CreateLink(ctx, code, destination, s.now())
		if err == nil {
			return s.store.GetLinkByID(ctx, id)
		}
		if !errors.Is(err, store.ErrSlugTaken) {
			return nil, err
		}
	}
	return nil, ErrSlugExhausted
}

func (s *Service) ResolveLink(ctx context.Context, slug string) (*store.Link, error) {
	return s.store.GetLinkBySlug(ctx, slug)
}

func (s *Service) UpdateDestination(ctx context.Context, id int64, destination string) error {
	if err := validateDestination(destination); err != nil {
		return err
	}
	return s.store.UpdateDestination(ctx, id, destination)
}

func validateDestination(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}
