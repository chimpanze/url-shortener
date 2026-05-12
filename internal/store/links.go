package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("store: not found")
	ErrSlugTaken = errors.New("store: slug already taken")
)

type Link struct {
	ID          int64
	Slug        string
	Destination string
	CreatedAt   time.Time
}

type LinkWithCount struct {
	Link
	Clicks int
}

func (s *Store) CreateLink(ctx context.Context, slug, destination string, createdAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO links (slug, destination, created_at) VALUES (?, ?, ?)`,
		slug, destination, createdAt.Unix(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrSlugTaken
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetLinkBySlug(ctx context.Context, slug string) (*Link, error) {
	return s.scanLink(s.db.QueryRowContext(ctx,
		`SELECT id, slug, destination, created_at FROM links WHERE slug = ?`, slug))
}

func (s *Store) GetLinkByID(ctx context.Context, id int64) (*Link, error) {
	return s.scanLink(s.db.QueryRowContext(ctx,
		`SELECT id, slug, destination, created_at FROM links WHERE id = ?`, id))
}

func (s *Store) ListLinksWithCounts(ctx context.Context) ([]LinkWithCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.slug, l.destination, l.created_at, COUNT(c.id)
		FROM links l
		LEFT JOIN clicks c ON c.link_id = l.id
		GROUP BY l.id
		ORDER BY l.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LinkWithCount
	for rows.Next() {
		var (
			lc LinkWithCount
			ts int64
		)
		if err := rows.Scan(&lc.ID, &lc.Slug, &lc.Destination, &ts, &lc.Clicks); err != nil {
			return nil, err
		}
		lc.CreatedAt = time.Unix(ts, 0)
		out = append(out, lc)
	}
	return out, rows.Err()
}

func (s *Store) UpdateDestination(ctx context.Context, id int64, destination string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE links SET destination = ? WHERE id = ?`, destination, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteLink(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM links WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) scanLink(row *sql.Row) (*Link, error) {
	var (
		l  Link
		ts int64
	)
	if err := row.Scan(&l.ID, &l.Slug, &l.Destination, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l.CreatedAt = time.Unix(ts, 0)
	return &l, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite surfaces UNIQUE failures with "UNIQUE constraint failed"
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
