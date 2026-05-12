package store

import (
	"context"
	"strings"
	"time"
)

type ClickRecord struct {
	LinkID    int64
	TS        time.Time
	Referer   string
	UserAgent string
}

func (s *Store) InsertClicks(ctx context.Context, recs []ClickRecord) error {
	if len(recs) == 0 {
		return nil
	}
	placeholders := make([]string, len(recs))
	args := make([]any, 0, len(recs)*4)
	for i, r := range recs {
		placeholders[i] = "(?, ?, ?, ?)"
		args = append(args, r.LinkID, r.TS.Unix(), r.Referer, r.UserAgent)
	}
	query := `INSERT INTO clicks (link_id, ts, referer, user_agent) VALUES ` +
		strings.Join(placeholders, ", ")
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) ListClicks(ctx context.Context, linkID int64, limit int) ([]ClickRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT link_id, ts, referer, user_agent FROM clicks
		 WHERE link_id = ? ORDER BY ts DESC LIMIT ?`,
		linkID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClickRecord
	for rows.Next() {
		var (
			r  ClickRecord
			ts int64
		)
		if err := rows.Scan(&r.LinkID, &ts, &r.Referer, &r.UserAgent); err != nil {
			return nil, err
		}
		r.TS = time.Unix(ts, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CountClicks(ctx context.Context, linkID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks WHERE link_id = ?`, linkID,
	).Scan(&n)
	return n, err
}
