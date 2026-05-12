package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrSessionExpired = errors.New("store: session expired")

type Session struct {
	Token     string
	CSRFToken string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, csrf_token, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		sess.Token, sess.CSRFToken, sess.CreatedAt.Unix(), sess.ExpiresAt.Unix(),
	)
	return err
}

func (s *Store) GetSession(ctx context.Context, token string) (*Session, error) {
	var (
		sess         Session
		createdUnix  int64
		expiresUnix  int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT token, csrf_token, created_at, expires_at FROM sessions WHERE token = ?`,
		token,
	).Scan(&sess.Token, &sess.CSRFToken, &createdUnix, &expiresUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = time.Unix(createdUnix, 0)
	sess.ExpiresAt = time.Unix(expiresUnix, 0)
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
	return err
}
