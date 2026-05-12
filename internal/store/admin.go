package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) SetAdminPasswordHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin (id, password_hash, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at`,
		hash, time.Now().Unix(),
	)
	return err
}

func (s *Store) GetAdminPasswordHash(ctx context.Context) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx,
		`SELECT password_hash FROM admin WHERE id = 1`,
	).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return h, err
}
