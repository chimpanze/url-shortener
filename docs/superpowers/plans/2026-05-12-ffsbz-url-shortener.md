# ffs.bz URL Shortener Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary Go URL shortener with SQLite storage, single-user password-protected admin UI, and per-click logging (timestamp, referer, user-agent).

**Architecture:** One Go process, chi router serving both public redirects (`/{slug}`) and admin (`/admin/*`). Async batched click writer keeps the redirect path off the DB. Templates and migrations embedded via `embed.FS`.

**Tech Stack:** Go (stdlib `database/sql`, `html/template`, `log/slog`, `embed`), `github.com/go-chi/chi/v5`, `modernc.org/sqlite` (pure-Go), `golang.org/x/crypto/bcrypt`.

**Module path:** `ffs.bz` (no remote — change later if hosted).

---

## File Map

```
ffs.bz/
├── go.mod                                       # module: ffs.bz
├── main.go                                      # subcommand dispatch
├── migrations/
│   ├── 0001_init.sql                            # schema
│   └── migrations.go                            # embed.FS for SQL files
├── internal/
│   ├── store/
│   │   ├── store.go                             # *sql.DB wrapper + migrations
│   │   ├── store_test.go
│   │   ├── links.go                             # Link CRUD
│   │   ├── links_test.go
│   │   ├── clicks.go                            # Click insert + queries
│   │   ├── clicks_test.go
│   │   ├── admin.go                             # admin password get/set
│   │   ├── admin_test.go
│   │   ├── sessions.go                          # session row CRUD
│   │   ├── sessions_test.go
│   │   └── testutil.go                          # newTestStore helper
│   ├── auth/
│   │   ├── password.go                          # bcrypt wrappers
│   │   ├── password_test.go
│   │   ├── session.go                           # SessionManager + middleware
│   │   └── session_test.go
│   ├── shortener/
│   │   ├── code.go                              # random base62 generator
│   │   ├── code_test.go
│   │   ├── service.go                           # CreateLink/ResolveLink + validation
│   │   └── service_test.go
│   ├── clicklog/
│   │   ├── logger.go                            # async buffered click writer
│   │   └── logger_test.go
│   └── web/
│       ├── server.go                            # chi router + embed FS
│       ├── render.go                            # template helpers + CSRF helper
│       ├── public.go                            # GET /{slug}
│       ├── public_test.go
│       ├── login.go                             # GET/POST login, POST logout
│       ├── login_test.go
│       ├── admin.go                             # admin handlers
│       ├── admin_test.go
│       ├── csrf.go                              # CSRF middleware
│       ├── csrf_test.go
│       ├── testutil.go                          # newTestServer helper
│       ├── templates/
│       │   ├── layout.html
│       │   ├── notfound.html
│       │   ├── login.html
│       │   ├── admin_list.html
│       │   ├── admin_new.html
│       │   ├── admin_edit.html
│       │   └── admin_detail.html
│       └── static/
│           └── style.css
└── docs/superpowers/specs/2026-05-12-ffsbz-url-shortener-design.md   # already exists
```

---

## Tasks

### Task 1: Bootstrap project

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `.gitignore`

- [ ] **Step 1: Initialize go.mod**

Run: `go mod init ffs.bz`

- [ ] **Step 2: Create .gitignore**

```
ffsbz
*.db
*.db-wal
*.db-shm
```

- [ ] **Step 3: Create a minimal main.go**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve", "set-password", "migrate":
		fmt.Fprintf(os.Stderr, "%s: not implemented yet\n", os.Args[1])
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ffsbz <serve|set-password|migrate> [flags]")
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./...`
Expected: no errors, produces a `ffs.bz` binary (named after the module). Delete it after.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore main.go
git commit -m "chore: bootstrap go module and main.go skeleton"
```

---

### Task 2: Store — DB connection, pragmas, migrations runner

**Files:**
- Create: `migrations/0001_init.sql`
- Create: `migrations/migrations.go`
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`
- Create: `internal/store/testutil.go`

`go:embed` cannot reference parent directories, so the SQL files are exposed
through a tiny `migrations` package whose Go file lives alongside them.

- [ ] **Step 1: Write the schema migration**

Create `migrations/0001_init.sql`:

```sql
CREATE TABLE links (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  slug        TEXT    NOT NULL UNIQUE,
  destination TEXT    NOT NULL,
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_links_created_at ON links(created_at DESC);

CREATE TABLE clicks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  ts         INTEGER NOT NULL,
  referer    TEXT    NOT NULL DEFAULT '',
  user_agent TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_clicks_link_ts ON clicks(link_id, ts DESC);

CREATE TABLE admin (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  password_hash TEXT    NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  token       TEXT    PRIMARY KEY,
  csrf_token  TEXT    NOT NULL,
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL
);

CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY
);
```

- [ ] **Step 2: Write a failing test for Open + Migrate**

Create `internal/store/store_test.go`:

```go
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	var count int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='links'`,
	).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("links table not created")
	}
}

func TestForeignKeysOn(t *testing.T) {
	s := newTestStore(t)
	var fk int
	if err := s.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys not ON (got %d)", fk)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/store/...`
Expected: FAIL — `Open` undefined, `newTestStore` undefined.

- [ ] **Step 4: Add modernc.org/sqlite dependency**

Run: `go get modernc.org/sqlite`

- [ ] **Step 5: Create the migrations package**

Create `migrations/migrations.go`:

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 6: Implement Open + Migrate**

Create `internal/store/store.go`:

```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"ffs.bz/migrations"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) DB() *sql.DB { return s.db }
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var version int
		fmt.Sscanf(name, "%d_", &version)

		var applied int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		sqlBytes, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES (?)`, version,
		); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 7: Create the test helper**

Create `internal/store/testutil.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
```

- [ ] **Step 8: Run tests**

Run: `go test ./internal/store/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add migrations/ internal/store/ go.mod go.sum
git commit -m "feat(store): add DB open + migration runner"
```

---

### Task 3: Store — Links CRUD

**Files:**
- Create: `internal/store/links.go`
- Create: `internal/store/links_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/links_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/...`
Expected: FAIL — all link symbols undefined.

- [ ] **Step 3: Implement Links**

Create `internal/store/links.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/links.go internal/store/links_test.go
git commit -m "feat(store): add link CRUD with unique-slug detection"
```

---

### Task 4: Store — Clicks insert + query

**Files:**
- Create: `internal/store/clicks.go`
- Create: `internal/store/clicks_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/clicks_test.go`:

```go
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
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/store/...`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Implement clicks**

Create `internal/store/clicks.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/clicks.go internal/store/clicks_test.go
git commit -m "feat(store): add batched click insert and list"
```

---

### Task 5: Store — Admin password

**Files:**
- Create: `internal/store/admin.go`
- Create: `internal/store/admin_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/admin_test.go`:

```go
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
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/store/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/store/admin.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/admin.go internal/store/admin_test.go
git commit -m "feat(store): add admin password get/set"
```

---

### Task 6: Store — Sessions

**Files:**
- Create: `internal/store/sessions.go`
- Create: `internal/store/sessions_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/sessions_test.go`:

```go
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
```

- [ ] **Step 2: Run tests**

Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/store/sessions.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/sessions.go internal/store/sessions_test.go
git commit -m "feat(store): add session CRUD with expiry"
```

---

### Task 7: Auth — Password (bcrypt)

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`

- [ ] **Step 1: Add bcrypt dependency**

Run: `go get golang.org/x/crypto/bcrypt`

- [ ] **Step 2: Write failing tests**

Create `internal/auth/password_test.go`:

```go
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if h == "hunter2" {
		t.Fatal("hash should not equal plaintext")
	}
	if err := VerifyPassword(h, "hunter2"); err != nil {
		t.Errorf("expected verify ok, got %v", err)
	}
	if err := VerifyPassword(h, "wrong"); err == nil {
		t.Errorf("expected verify to fail on wrong password")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/auth/...`
Expected: FAIL.

- [ ] **Step 4: Implement**

Create `internal/auth/password.go`:

```go
package auth

import "golang.org/x/crypto/bcrypt"

const passwordCost = 12

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), passwordCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func VerifyPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
```

- [ ] **Step 5: Run tests**

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/password.go internal/auth/password_test.go go.mod go.sum
git commit -m "feat(auth): add bcrypt password hash + verify"
```

---

### Task 8: Shortener — Random code generation

**Files:**
- Create: `internal/shortener/code.go`
- Create: `internal/shortener/code_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/shortener/code_test.go`:

```go
package shortener

import (
	"strings"
	"testing"
)

func TestRandomCodeFormat(t *testing.T) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ123456789"

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := RandomCode(6)
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 6 {
			t.Fatalf("len = %d, want 6", len(c))
		}
		for _, r := range c {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("unexpected char %q in %q", r, c)
			}
		}
		seen[c] = true
	}
	if len(seen) < 150 {
		t.Fatalf("only %d unique codes in 200 attempts — bad entropy", len(seen))
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/shortener/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/shortener/code.go`:

```go
package shortener

import (
	"crypto/rand"
	"math/big"
)

// Alphabet: base62 minus 0, O, I, l.
const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ123456789"

func RandomCode(length int) (string, error) {
	max := big.NewInt(int64(len(alphabet)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}
```

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/shortener/code.go internal/shortener/code_test.go
git commit -m "feat(shortener): add random base62 code generator"
```

---

### Task 9: Shortener — Service (validation + CreateLink)

**Files:**
- Create: `internal/shortener/service.go`
- Create: `internal/shortener/service_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/shortener/service_test.go`:

```go
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
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/shortener/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/shortener/service.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/shortener/service.go internal/shortener/service_test.go
git commit -m "feat(shortener): add link creation with validation and random-slug retry"
```

---

### Task 10: Click logger (async)

**Files:**
- Create: `internal/clicklog/logger.go`
- Create: `internal/clicklog/logger_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/clicklog/logger_test.go`:

```go
package clicklog

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ffs.bz/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLoggerWritesEvents(t *testing.T) {
	s := newStore(t)
	id, _ := s.CreateLink(context.Background(), "x", "https://x.example", time.Now())

	lg := New(s, Config{BufferSize: 16, FlushMaxBatch: 4, FlushInterval: 20 * time.Millisecond})
	lg.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		lg.Shutdown(ctx)
	}()

	for i := 0; i < 10; i++ {
		lg.Enqueue(Event{LinkID: id, TS: time.Unix(int64(i), 0), Referer: "r", UserAgent: "ua"})
	}

	// Wait until all 10 are written, with a timeout.
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, _ := s.CountClicks(context.Background(), id)
		if n == 10 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected 10 clicks, got %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoggerOverflowDropsEvents(t *testing.T) {
	s := newStore(t)
	id, _ := s.CreateLink(context.Background(), "x", "https://x.example", time.Now())

	// Tiny buffer + no worker started, so every enqueue past 2 must be dropped.
	lg := New(s, Config{BufferSize: 2, FlushMaxBatch: 1, FlushInterval: time.Hour})
	// Intentionally do NOT Start() — fill the channel.

	dropped := int64(0)
	for i := 0; i < 10; i++ {
		if !lg.tryEnqueue(Event{LinkID: id, TS: time.Now()}) {
			atomic.AddInt64(&dropped, 1)
		}
	}
	if dropped == 0 {
		t.Fatal("expected some drops, got none")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/clicklog/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/clicklog/logger.go`:

```go
package clicklog

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"ffs.bz/internal/store"
)

type Event struct {
	LinkID    int64
	TS        time.Time
	Referer   string
	UserAgent string
}

type Config struct {
	BufferSize    int
	FlushMaxBatch int
	FlushInterval time.Duration
}

func DefaultConfig() Config {
	return Config{BufferSize: 1024, FlushMaxBatch: 64, FlushInterval: 200 * time.Millisecond}
}

type Logger struct {
	store   *store.Store
	cfg     Config
	ch      chan Event
	done    chan struct{}
	dropped int64
}

func New(s *store.Store, cfg Config) *Logger {
	if cfg.BufferSize <= 0 {
		cfg = DefaultConfig()
	}
	return &Logger{
		store: s,
		cfg:   cfg,
		ch:    make(chan Event, cfg.BufferSize),
		done:  make(chan struct{}),
	}
}

func (l *Logger) Start() {
	go l.run()
}

func (l *Logger) Enqueue(e Event) {
	if !l.tryEnqueue(e) {
		atomic.AddInt64(&l.dropped, 1)
	}
}

func (l *Logger) tryEnqueue(e Event) bool {
	select {
	case l.ch <- e:
		return true
	default:
		return false
	}
}

func (l *Logger) Shutdown(ctx context.Context) {
	close(l.ch)
	select {
	case <-l.done:
	case <-ctx.Done():
	}
}

func (l *Logger) run() {
	defer close(l.done)
	ticker := time.NewTicker(l.cfg.FlushInterval)
	defer ticker.Stop()
	dropReport := time.NewTicker(time.Minute)
	defer dropReport.Stop()

	var batch []Event
	flush := func() {
		if len(batch) == 0 {
			return
		}
		recs := make([]store.ClickRecord, len(batch))
		for i, e := range batch {
			recs[i] = store.ClickRecord{
				LinkID: e.LinkID, TS: e.TS, Referer: e.Referer, UserAgent: e.UserAgent,
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := l.store.InsertClicks(ctx, recs); err != nil {
			slog.Error("clicklog flush failed", "err", err, "n", len(batch))
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= l.cfg.FlushMaxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-dropReport.C:
			if d := atomic.SwapInt64(&l.dropped, 0); d > 0 {
				slog.Warn("clicklog dropped events", "n", d)
			}
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/clicklog/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/clicklog/
git commit -m "feat(clicklog): add async batched click writer with overflow drop"
```

---

### Task 11: Auth — Session manager + middleware

**Files:**
- Create: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/auth/session_test.go`:

```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ffs.bz/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLoginCreatesSession(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	hash, _ := HashPassword("pw")
	_ = s.SetAdminPasswordHash(ctx, hash)

	mgr := NewSessionManager(s, SessionConfig{TTL: time.Hour, CookieName: "ffsbz_session"})
	w := httptest.NewRecorder()
	if err := mgr.Login(ctx, w, "pw"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 || cookies[0].Name != "ffsbz_session" {
		t.Fatal("expected session cookie")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s := newStore(t)
	hash, _ := HashPassword("pw")
	_ = s.SetAdminPasswordHash(context.Background(), hash)
	mgr := NewSessionManager(s, SessionConfig{TTL: time.Hour, CookieName: "ffsbz_session"})
	w := httptest.NewRecorder()
	if err := mgr.Login(context.Background(), w, "wrong"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRequireAuthMiddleware(t *testing.T) {
	s := newStore(t)
	hash, _ := HashPassword("pw")
	_ = s.SetAdminPasswordHash(context.Background(), hash)
	mgr := NewSessionManager(s, SessionConfig{TTL: time.Hour, CookieName: "ffsbz_session", LoginPath: "/admin/login"})

	// Authenticated.
	w := httptest.NewRecorder()
	_ = mgr.Login(context.Background(), w, "pw")
	cookie := w.Result().Cookies()[0]

	called := false
	handler := mgr.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if !called {
		t.Error("expected handler called")
	}

	// Unauthenticated.
	req2 := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rw2 := httptest.NewRecorder()
	handler.ServeHTTP(rw2, req2)
	if rw2.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rw2.Code)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/auth/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/auth/session.go`:

```go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"ffs.bz/internal/store"
)

var ErrLoginFailed = errors.New("auth: invalid password")

type SessionConfig struct {
	CookieName    string
	TTL           time.Duration
	SecureCookies bool
	LoginPath     string
}

type ctxKey int

const sessionCtxKey ctxKey = 1

type SessionManager struct {
	store *store.Store
	cfg   SessionConfig
}

func NewSessionManager(s *store.Store, cfg SessionConfig) *SessionManager {
	if cfg.CookieName == "" {
		cfg.CookieName = "ffsbz_session"
	}
	if cfg.TTL == 0 {
		cfg.TTL = 7 * 24 * time.Hour
	}
	if cfg.LoginPath == "" {
		cfg.LoginPath = "/admin/login"
	}
	return &SessionManager{store: s, cfg: cfg}
}

func (m *SessionManager) Login(ctx context.Context, w http.ResponseWriter, password string) error {
	hash, err := m.store.GetAdminPasswordHash(ctx)
	if err != nil {
		return ErrLoginFailed
	}
	if err := VerifyPassword(hash, password); err != nil {
		return ErrLoginFailed
	}
	token, err := randomHex(32)
	if err != nil {
		return err
	}
	csrf, err := randomHex(32)
	if err != nil {
		return err
	}
	now := time.Now()
	sess := store.Session{
		Token: token, CSRFToken: csrf,
		CreatedAt: now, ExpiresAt: now.Add(m.cfg.TTL),
	}
	if err := m.store.CreateSession(ctx, sess); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    token,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   m.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	return nil
}

func (m *SessionManager) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(m.cfg.CookieName); err == nil {
		_ = m.store.DeleteSession(ctx, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (m *SessionManager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(m.cfg.CookieName)
		if err != nil {
			http.Redirect(w, r, m.cfg.LoginPath, http.StatusSeeOther)
			return
		}
		sess, err := m.store.GetSession(r.Context(), c.Value)
		if err != nil {
			http.Redirect(w, r, m.cfg.LoginPath, http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SessionFromContext(ctx context.Context) (*store.Session, bool) {
	v, ok := ctx.Value(sessionCtxKey).(*store.Session)
	return v, ok
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/session.go internal/auth/session_test.go
git commit -m "feat(auth): add session manager with login/logout/require-auth middleware"
```

---

### Task 12: Web — Server skeleton + templates + static

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/render.go`
- Create: `internal/web/testutil.go`
- Create: `internal/web/templates/layout.html`
- Create: `internal/web/templates/notfound.html`
- Create: `internal/web/static/style.css`

- [ ] **Step 1: Add chi dependency**

Run: `go get github.com/go-chi/chi/v5`

- [ ] **Step 2: Create base template**

Create `internal/web/templates/layout.html`:

```html
{{define "layout"}}<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{.Title}} · ffs.bz</title>
  <link rel="stylesheet" href="/admin/static/style.css">
</head>
<body>
  <header><a href="/admin">ffs.bz admin</a></header>
  <main>{{template "content" .}}</main>
</body>
</html>{{end}}
```

Create `internal/web/templates/notfound.html`:

```html
{{define "content"}}<h1>Not found</h1><p>This short link doesn't exist.</p>{{end}}
```

Create `internal/web/static/style.css`:

```css
body { font-family: system-ui, sans-serif; max-width: 760px; margin: 2rem auto; padding: 0 1rem; }
header a { font-weight: bold; text-decoration: none; color: inherit; }
table { width: 100%; border-collapse: collapse; margin: 1rem 0; }
th, td { padding: .4rem .6rem; border-bottom: 1px solid #ddd; text-align: left; }
.error { color: #b00020; }
form label { display: block; margin: .5rem 0 .2rem; }
form input[type=text], form input[type=password] { width: 100%; padding: .4rem; font-size: 1rem; }
button { padding: .5rem 1rem; font-size: 1rem; cursor: pointer; }
```

- [ ] **Step 3: Create the test helper**

Create `internal/web/testutil.go`:

```go
package web

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/shortener"
	"ffs.bz/internal/store"
)

type testEnv struct {
	store    *store.Store
	server   *Server
	sessions *auth.SessionManager
	clicks   *clicklog.Logger
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	mgr := auth.NewSessionManager(s, auth.SessionConfig{
		CookieName: "ffsbz_session", TTL: time.Hour, LoginPath: "/admin/login",
	})
	cl := clicklog.New(s, clicklog.Config{
		BufferSize: 16, FlushMaxBatch: 4, FlushInterval: 20 * time.Millisecond,
	})
	cl.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cl.Shutdown(ctx)
	})

	svr := NewServer(Deps{
		Store:     s,
		Shortener: shortener.New(s),
		Sessions:  mgr,
		Clicks:    cl,
	})
	return &testEnv{store: s, server: svr, sessions: mgr, clicks: cl}
}
```

- [ ] **Step 4: Write a failing test for the server skeleton**

Create `internal/web/server_test.go`:

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterServesStaticCSS(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/static/style.css", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if rw.Body.Len() == 0 {
		t.Fatal("empty body")
	}
}

func TestUnknownSlugReturns404(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Errorf("status = %d", rw.Code)
	}
}
```

- [ ] **Step 5: Run tests**

Expected: FAIL — `Server`, `Deps`, `NewServer`, `Router` undefined.

- [ ] **Step 6: Implement render helpers**

Create `internal/web/render.go`:

```go
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

type templateData struct {
	Title string
	CSRF  string
	Flash string
	Data  any
}

func loadTemplates() (map[string]*template.Template, error) {
	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, err
	}
	layout, err := templatesFS.ReadFile("templates/layout.html")
	if err != nil {
		return nil, err
	}
	out := map[string]*template.Template{}
	for _, e := range entries {
		if e.Name() == "layout.html" {
			continue
		}
		b, err := templatesFS.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, err
		}
		t, err := template.New(e.Name()).Parse(string(layout))
		if err != nil {
			return nil, err
		}
		if _, err := t.Parse(string(b)); err != nil {
			return nil, err
		}
		out[e.Name()] = t
	}
	return out, nil
}

func (s *Server) render(w http.ResponseWriter, name string, status int, data templateData) {
	t, ok := s.templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("template %s not found", name), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func staticFileSystem() http.FileSystem {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}
```

- [ ] **Step 7: Implement server**

Create `internal/web/server.go`:

```go
package web

import (
	"html/template"
	"net/http"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/shortener"
	"ffs.bz/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Store     *store.Store
	Shortener *shortener.Service
	Sessions  *auth.SessionManager
	Clicks    *clicklog.Logger
}

type Server struct {
	deps      Deps
	templates map[string]*template.Template
	router    chi.Router
}

func NewServer(d Deps) *Server {
	tpls, err := loadTemplates()
	if err != nil {
		panic(err)
	}
	s := &Server{deps: d, templates: tpls}
	s.router = s.buildRouter()
	return s
}

func (s *Server) Router() http.Handler { return s.router }

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)

	r.Handle("/admin/static/*", http.StripPrefix("/admin/static/", http.FileServer(staticFileSystem())))

	r.Get("/{slug}", s.handleRedirect)
	r.Get("/", s.handleHome)

	return r
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// handleRedirect is implemented in public.go (Task 13).
```

Add a temporary stub for `handleRedirect` so the package compiles. Create `internal/web/public.go`:

```go
package web

import "net/http"

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
```

- [ ] **Step 8: Run tests**

Run: `go test ./internal/web/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/web/ go.mod go.sum
git commit -m "feat(web): add chi router skeleton with embedded templates and static"
```

---

### Task 13: Web — Public redirect handler

**Files:**
- Modify: `internal/web/public.go`
- Create: `internal/web/public_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/web/public_test.go`:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRedirectHit(t *testing.T) {
	env := newTestEnv(t)
	id, err := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = id

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req.Header.Set("Referer", "https://r.example")
	req.Header.Set("User-Agent", "test-ua")
	rw := httptest.NewRecorder()

	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rw.Code)
	}
	if loc := rw.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("location = %q", loc)
	}

	// Click should land asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, _ := env.store.CountClicks(context.Background(), id)
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected 1 click, got %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRedirectMiss(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Errorf("status = %d", rw.Code)
	}
}
```

- [ ] **Step 2: Run tests**

Expected: FAIL on `TestRedirectHit` (currently always 404).

- [ ] **Step 3: Implement**

Replace `internal/web/public.go` with:

```go
package web

import (
	"errors"
	"net/http"
	"time"

	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	link, err := s.deps.Shortener.ResolveLink(r.Context(), slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.render(w, "notfound.html", http.StatusNotFound, templateData{Title: "Not found"})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, link.Destination, http.StatusFound)
	s.deps.Clicks.Enqueue(clicklog.Event{
		LinkID:    link.ID,
		TS:        time.Now(),
		Referer:   r.Header.Get("Referer"),
		UserAgent: r.Header.Get("User-Agent"),
	})
}
```

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/web/public.go internal/web/public_test.go
git commit -m "feat(web): add public redirect handler with async click enqueue"
```

---

### Task 14: Web — Login/Logout handlers

**Files:**
- Create: `internal/web/login.go`
- Create: `internal/web/login_test.go`
- Create: `internal/web/templates/login.html`

- [ ] **Step 1: Create the template**

Create `internal/web/templates/login.html`:

```html
{{define "content"}}
<h1>Admin login</h1>
{{if .Flash}}<p class="error">{{.Flash}}</p>{{end}}
<form method="post" action="/admin/login">
  <label>Password<input type="password" name="password" autocomplete="current-password" autofocus></label>
  <button type="submit">Log in</button>
</form>
{{end}}
```

- [ ] **Step 2: Write failing tests**

Create `internal/web/login_test.go`:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ffs.bz/internal/auth"
)

func setAdminPassword(t *testing.T, env *testEnv, pw string) {
	t.Helper()
	hash, _ := auth.HashPassword(pw)
	if err := env.store.SetAdminPasswordHash(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
}

func TestLoginPageRenders(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "Admin login") {
		t.Errorf("expected login page content")
	}
}

func TestLoginSuccess(t *testing.T) {
	env := newTestEnv(t)
	setAdminPassword(t, env, "pw")

	form := url.Values{"password": []string{"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	cookies := rw.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "ffsbz_session" {
		t.Fatalf("expected session cookie, got %+v", cookies)
	}
}

func TestLoginWrong(t *testing.T) {
	env := newTestEnv(t)
	setAdminPassword(t, env, "pw")

	form := url.Values{"password": []string{"nope"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rw.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	env := newTestEnv(t)
	setAdminPassword(t, env, "pw")

	// Log in to obtain a cookie.
	form := url.Values{"password": []string{"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	cookie := rw.Result().Cookies()[0]

	logoutReq := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRW := httptest.NewRecorder()
	env.server.Router().ServeHTTP(logoutRW, logoutReq)
	if logoutRW.Code != http.StatusSeeOther {
		t.Errorf("logout status = %d", logoutRW.Code)
	}

	// Cookie deletion: max-age < 0 or expires in past.
	gone := logoutRW.Result().Cookies()
	if len(gone) == 0 || gone[0].MaxAge >= 0 {
		t.Errorf("expected cookie cleared, got %+v", gone)
	}
}
```

- [ ] **Step 3: Run tests**

Expected: FAIL — routes don't exist.

- [ ] **Step 4: Implement login handlers**

Create `internal/web/login.go`:

```go
package web

import (
	"errors"
	"net/http"

	"ffs.bz/internal/auth"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", http.StatusOK, templateData{Title: "Login"})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	pw := r.PostForm.Get("password")
	if err := s.deps.Sessions.Login(r.Context(), w, pw); err != nil {
		if errors.Is(err, auth.ErrLoginFailed) {
			s.render(w, "login.html", http.StatusUnauthorized, templateData{
				Title: "Login", Flash: "Invalid password",
			})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.deps.Sessions.Logout(r.Context(), w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}
```

- [ ] **Step 5: Wire routes into `server.go`**

Modify `internal/web/server.go` — replace the `buildRouter` body so it now includes the login/logout routes. Replace:

```go
	r.Handle("/admin/static/*", http.StripPrefix("/admin/static/", http.FileServer(staticFileSystem())))

	r.Get("/{slug}", s.handleRedirect)
	r.Get("/", s.handleHome)
```

with:

```go
	r.Handle("/admin/static/*", http.StripPrefix("/admin/static/", http.FileServer(staticFileSystem())))

	r.Route("/admin", func(r chi.Router) {
		r.Get("/login", s.handleLoginGet)
		r.Post("/login", s.handleLoginPost)
		r.Post("/logout", s.handleLogout)
	})

	r.Get("/", s.handleHome)
	r.Get("/{slug}", s.handleRedirect)
```

- [ ] **Step 6: Run tests**

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/web/login.go internal/web/login_test.go internal/web/server.go internal/web/templates/login.html
git commit -m "feat(web): add login/logout handlers"
```

---

### Task 15: Web — CSRF middleware

**Files:**
- Create: `internal/web/csrf.go`
- Create: `internal/web/csrf_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/web/csrf_test.go`:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ffs.bz/internal/store"
)

func authCookie(t *testing.T, env *testEnv) *http.Cookie {
	t.Helper()
	setAdminPassword(t, env, "pw")
	form := url.Values{"password": []string{"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	return rw.Result().Cookies()[0]
}

// Confirms CSRF middleware rejects state-changing requests without a token.
func TestCSRFMissingTokenRejected(t *testing.T) {
	env := newTestEnv(t)
	// Pre-create a link so /admin/links/{id}/delete is targetable.
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)

	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+itoa(id)+"/delete", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rw.Code)
	}
	_ = store.ErrNotFound // ensure import retained
}

func TestCSRFValidTokenAccepted(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)

	sess, err := env.store.GetSession(context.Background(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"csrf_token": []string{sess.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+itoa(id)+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rw.Code)
	}
}

func itoa(n int64) string {
	return strings.TrimSpace(string([]byte{
		'0' + byte((n/100)%10), '0' + byte((n/10)%10), '0' + byte(n%10),
	}))
}
```

Note: `itoa` is intentionally tiny because IDs in these tests are small ints. If you prefer, use `strconv.FormatInt(id, 10)` and import strconv.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/web/...`
Expected: FAIL — admin delete route + CSRF middleware not yet wired.

Defer to Task 18 (admin delete). For Task 15, deliver the CSRF middleware itself with a tiny self-contained test instead. Replace the above test file with this version, which doesn't depend on admin handlers existing yet:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/store"
)

func TestCSRFMiddlewareRejectsMissing(t *testing.T) {
	env := newTestEnv(t)
	hash, _ := auth.HashPassword("pw")
	_ = env.store.SetAdminPasswordHash(context.Background(), hash)

	// Manually create a session row.
	sess := store.Session{
		Token: "tok-1", CSRFToken: "csrf-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = env.store.CreateSession(context.Background(), sess)

	handler := env.server.deps.Sessions.RequireAuth(env.server.csrfProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/admin/anything", nil)
	req.AddCookie(&http.Cookie{Name: "ffsbz_session", Value: "tok-1"})
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rw.Code)
	}
}

func TestCSRFMiddlewareAcceptsMatching(t *testing.T) {
	env := newTestEnv(t)
	hash, _ := auth.HashPassword("pw")
	_ = env.store.SetAdminPasswordHash(context.Background(), hash)

	sess := store.Session{
		Token: "tok-2", CSRFToken: "csrf-2",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = env.store.CreateSession(context.Background(), sess)

	handler := env.server.deps.Sessions.RequireAuth(env.server.csrfProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	form := url.Values{"csrf_token": []string{"csrf-2"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/anything", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ffsbz_session", Value: "tok-2"})
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rw.Code)
	}
}
```

- [ ] **Step 3: Implement middleware**

Create `internal/web/csrf.go`:

```go
package web

import (
	"net/http"

	"ffs.bz/internal/auth"
)

func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := auth.SessionFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		token := r.PostForm.Get("csrf_token")
		if token == "" || token != sess.CSRFToken {
			http.Error(w, "csrf token mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/web/...`
Expected: PASS for csrf tests (other tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/web/csrf.go internal/web/csrf_test.go
git commit -m "feat(web): add CSRF middleware for state-changing admin requests"
```

---

### Task 16: Web — Admin list

**Files:**
- Create: `internal/web/admin.go`
- Create: `internal/web/admin_test.go`
- Create: `internal/web/templates/admin_list.html`

- [ ] **Step 1: Create template**

Create `internal/web/templates/admin_list.html`:

```html
{{define "content"}}
<h1>Links</h1>
<p><a href="/admin/new">+ New link</a> · <form method="post" action="/admin/logout" style="display:inline"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button>Log out</button></form></p>
{{with .Data}}
<table>
  <tr><th>Slug</th><th>Destination</th><th>Clicks</th><th>Created</th><th></th></tr>
  {{range .}}
  <tr>
    <td><a href="/admin/links/{{.ID}}">{{.Slug}}</a></td>
    <td>{{.Destination}}</td>
    <td>{{.Clicks}}</td>
    <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
    <td></td>
  </tr>
  {{end}}
</table>
{{end}}
{{end}}
```

- [ ] **Step 2: Write failing test**

Create `internal/web/admin_test.go`:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminListShowsLinks(t *testing.T) {
	env := newTestEnv(t)
	_, _ = env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "abc") || !strings.Contains(body, "https://example.com") {
		t.Errorf("missing link in body: %s", body)
	}
}

func TestAdminListRedirectsWithoutSession(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Errorf("status = %d", rw.Code)
	}
}
```

Note: `authCookie` was defined in Task 14's login_test.go.

- [ ] **Step 3: Run tests**

Expected: FAIL — `/admin` not wired with `RequireAuth`.

- [ ] **Step 4: Implement handler + wire routes**

Create `internal/web/admin.go`:

```go
package web

import (
	"net/http"

	"ffs.bz/internal/auth"
)

func (s *Server) handleAdminList(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Store.ListLinksWithCounts(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_list.html", http.StatusOK, templateData{
		Title: "Links", CSRF: csrf, Data: list,
	})
}
```

Modify `internal/web/server.go` — replace the `r.Route("/admin", ...)` block with one that mounts protected routes:

```go
	r.Route("/admin", func(r chi.Router) {
		r.Get("/login", s.handleLoginGet)
		r.Post("/login", s.handleLoginPost)

		r.Group(func(r chi.Router) {
			r.Use(s.deps.Sessions.RequireAuth)
			r.Get("/", s.handleAdminList)
			r.With(s.csrfProtect).Post("/logout", s.handleLogout)
		})
	})
```

- [ ] **Step 5: Run tests**

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/web/admin.go internal/web/admin_test.go internal/web/templates/admin_list.html internal/web/server.go
git commit -m "feat(web): add admin list page"
```

---

### Task 17: Web — Admin new (create)

**Files:**
- Modify: `internal/web/admin.go`
- Modify: `internal/web/admin_test.go`
- Modify: `internal/web/server.go`
- Create: `internal/web/templates/admin_new.html`

- [ ] **Step 1: Create template**

Create `internal/web/templates/admin_new.html`:

```html
{{define "content"}}
<h1>New link</h1>
{{if .Flash}}<p class="error">{{.Flash}}</p>{{end}}
<form method="post" action="/admin/new">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Destination URL<input type="text" name="destination" value="{{with .Data}}{{.Destination}}{{end}}" required></label>
  <label>Custom slug (optional)<input type="text" name="slug" value="{{with .Data}}{{.Slug}}{{end}}"></label>
  <button type="submit">Create</button>
</form>
{{end}}
```

- [ ] **Step 2: Write failing tests**

Append to `internal/web/admin_test.go`:

```go
func TestAdminNewPage(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	req := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "New link") {
		t.Errorf("missing form")
	}
}

func TestAdminNewCreatesLinkWithCustomSlug(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"slug":        []string{"blog"},
		"destination": []string{"https://example.com"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	link, err := env.store.GetLinkBySlug(context.Background(), "blog")
	if err != nil {
		t.Fatalf("link not created: %v", err)
	}
	if link.Destination != "https://example.com" {
		t.Errorf("destination = %q", link.Destination)
	}
}

func TestAdminNewValidationError(t *testing.T) {
	env := newTestEnv(t)
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"destination": []string{"not-a-url"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rw.Code)
	}
}
```

You'll also need these imports near the top: `"net/url"` (likely already there). Confirm with `go test` errors.

- [ ] **Step 3: Run tests**

Expected: FAIL.

- [ ] **Step 4: Implement**

Append to `internal/web/admin.go`:

```go
type newLinkForm struct {
	Slug        string
	Destination string
}

func (s *Server) handleAdminNewGet(w http.ResponseWriter, r *http.Request) {
	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_new.html", http.StatusOK, templateData{
		Title: "New link", CSRF: csrf, Data: newLinkForm{},
	})
}

func (s *Server) handleAdminNewPost(w http.ResponseWriter, r *http.Request) {
	// ParseForm already called by csrfProtect.
	slug := r.PostForm.Get("slug")
	dest := r.PostForm.Get("destination")

	_, err := s.deps.Shortener.CreateLink(r.Context(), slug, dest)
	if err != nil {
		csrf := ""
		if sess, ok := auth.SessionFromContext(r.Context()); ok {
			csrf = sess.CSRFToken
		}
		s.render(w, "admin_new.html", http.StatusBadRequest, templateData{
			Title: "New link", CSRF: csrf, Flash: err.Error(),
			Data: newLinkForm{Slug: slug, Destination: dest},
		})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
```

Modify `internal/web/server.go` — inside the auth-protected `Group`, add:

```go
			r.Get("/new", s.handleAdminNewGet)
			r.With(s.csrfProtect).Post("/new", s.handleAdminNewPost)
```

- [ ] **Step 5: Run tests**

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/web/admin.go internal/web/admin_test.go internal/web/server.go internal/web/templates/admin_new.html
git commit -m "feat(web): add admin create-link page"
```

---

### Task 18: Web — Admin detail (clicks) + delete

**Files:**
- Modify: `internal/web/admin.go`
- Modify: `internal/web/admin_test.go`
- Modify: `internal/web/server.go`
- Create: `internal/web/templates/admin_detail.html`

- [ ] **Step 1: Create template**

Create `internal/web/templates/admin_detail.html`:

```html
{{define "content"}}
{{with .Data}}
<p><a href="/admin">← back</a></p>
<h1>{{.Link.Slug}}</h1>
<p>Destination: <code>{{.Link.Destination}}</code></p>
<p>Total clicks: <strong>{{.Total}}</strong></p>

<h2>Actions</h2>
<form method="post" action="/admin/links/{{.Link.ID}}/delete" onsubmit="return confirm('Delete this link?');">
  <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
  <button type="submit">Delete link</button>
</form>
<p><a href="/admin/links/{{.Link.ID}}/edit">Edit destination</a></p>

<h2>Recent clicks</h2>
<table>
  <tr><th>When</th><th>Referer</th><th>User-Agent</th></tr>
  {{range .Clicks}}
  <tr>
    <td>{{.TS.Format "2006-01-02 15:04:05"}}</td>
    <td>{{.Referer}}</td>
    <td>{{.UserAgent}}</td>
  </tr>
  {{end}}
</table>
{{end}}
{{end}}
```

- [ ] **Step 2: Write failing tests**

Append to `internal/web/admin_test.go`:

```go
func TestAdminDetailShowsClicks(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	_ = env.store.InsertClicks(context.Background(), []store.ClickRecord{
		{LinkID: id, TS: time.Now(), Referer: "https://r.example", UserAgent: "ua-1"},
	})
	cookie := authCookie(t, env)

	req := httptest.NewRequest(http.MethodGet, "/admin/links/"+strconv.FormatInt(id, 10), nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "ua-1") || !strings.Contains(body, "r.example") {
		t.Errorf("missing click data: %s", body)
	}
}

func TestAdminDelete(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{"csrf_token": []string{sess.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+strconv.FormatInt(id, 10)+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)

	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	if _, err := env.store.GetLinkByID(context.Background(), id); err != store.ErrNotFound {
		t.Errorf("link not deleted (err=%v)", err)
	}
}
```

You'll need to add `"strconv"` and `"ffs.bz/internal/store"` to the imports.

- [ ] **Step 3: Run tests**

Expected: FAIL.

- [ ] **Step 4: Implement**

Append to `internal/web/admin.go`:

```go
import (
	"errors"
	"strconv"

	"ffs.bz/internal/store"
)

// (Merge imports with the existing import block.)

type adminDetailData struct {
	Link   *store.Link
	Total  int
	Clicks []store.ClickRecord
}

func (s *Server) parseID(r *http.Request) (int64, error) {
	// chi already extracted it via URL param.
	v := chiURLParam(r, "id")
	return strconv.ParseInt(v, 10, 64)
}

func (s *Server) handleAdminDetail(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	link, err := s.deps.Store.GetLinkByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	total, _ := s.deps.Store.CountClicks(r.Context(), id)
	clicks, _ := s.deps.Store.ListClicks(r.Context(), id, 100)

	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_detail.html", http.StatusOK, templateData{
		Title: link.Slug, CSRF: csrf,
		Data: adminDetailData{Link: link, Total: total, Clicks: clicks},
	})
}

func (s *Server) handleAdminDelete(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.deps.Store.DeleteLink(r.Context(), id); err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
```

Add a small helper so the file doesn't import chi directly (keeping the chi dependency to `server.go`). Add to `internal/web/server.go`:

```go
func chiURLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
```

Modify `internal/web/server.go` — inside the auth-protected `Group`, add:

```go
			r.Get("/links/{id}", s.handleAdminDetail)
			r.With(s.csrfProtect).Post("/links/{id}/delete", s.handleAdminDelete)
```

- [ ] **Step 5: Run tests**

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/web/admin.go internal/web/admin_test.go internal/web/server.go internal/web/templates/admin_detail.html
git commit -m "feat(web): add admin detail page and delete action"
```

---

### Task 19: Web — Admin edit destination

**Files:**
- Modify: `internal/web/admin.go`
- Modify: `internal/web/admin_test.go`
- Modify: `internal/web/server.go`
- Create: `internal/web/templates/admin_edit.html`

- [ ] **Step 1: Create template**

Create `internal/web/templates/admin_edit.html`:

```html
{{define "content"}}
{{with .Data}}
<p><a href="/admin/links/{{.ID}}">← back</a></p>
<h1>Edit {{.Slug}}</h1>
{{if $.Flash}}<p class="error">{{$.Flash}}</p>{{end}}
<form method="post" action="/admin/links/{{.ID}}/edit">
  <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
  <label>Destination URL<input type="text" name="destination" value="{{.Destination}}" required></label>
  <button type="submit">Save</button>
</form>
{{end}}
{{end}}
```

- [ ] **Step 2: Write failing tests**

Append to `internal/web/admin_test.go`:

```go
func TestAdminEditPage(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://old.example", time.Now())
	cookie := authCookie(t, env)
	req := httptest.NewRequest(http.MethodGet, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", nil)
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "https://old.example") {
		t.Errorf("expected old destination prefilled")
	}
}

func TestAdminEditPost(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://old.example", time.Now())
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"destination": []string{"https://new.example"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rw.Code)
	}
	link, _ := env.store.GetLinkByID(context.Background(), id)
	if link.Destination != "https://new.example" {
		t.Errorf("destination = %q", link.Destination)
	}
}

func TestAdminEditRejectsBadURL(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.store.CreateLink(context.Background(), "abc", "https://example.com", time.Now())
	cookie := authCookie(t, env)
	sess, _ := env.store.GetSession(context.Background(), cookie.Value)

	form := url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"destination": []string{"javascript:alert(1)"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rw.Code)
	}
}
```

- [ ] **Step 3: Run tests**

Expected: FAIL.

- [ ] **Step 4: Add a validation helper and edit handler**

Append to `internal/shortener/service.go`:

```go
func (s *Service) UpdateDestination(ctx context.Context, id int64, destination string) error {
	if err := validateDestination(destination); err != nil {
		return err
	}
	return s.store.UpdateDestination(ctx, id, destination)
}
```

Append to `internal/web/admin.go`:

```go
func (s *Server) handleAdminEditGet(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	link, err := s.deps.Store.GetLinkByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_edit.html", http.StatusOK, templateData{
		Title: "Edit " + link.Slug, CSRF: csrf, Data: link,
	})
}

func (s *Server) handleAdminEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	dest := r.PostForm.Get("destination")
	if err := s.deps.Shortener.UpdateDestination(r.Context(), id, dest); err != nil {
		link, _ := s.deps.Store.GetLinkByID(r.Context(), id)
		csrf := ""
		if sess, ok := auth.SessionFromContext(r.Context()); ok {
			csrf = sess.CSRFToken
		}
		// Show error but keep the user's input in the form.
		if link != nil {
			link.Destination = dest
		}
		s.render(w, "admin_edit.html", http.StatusBadRequest, templateData{
			Title: "Edit", CSRF: csrf, Flash: err.Error(), Data: link,
		})
		return
	}
	http.Redirect(w, r, "/admin/links/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
```

Modify `internal/web/server.go` — inside the auth-protected `Group`, add:

```go
			r.Get("/links/{id}/edit", s.handleAdminEditGet)
			r.With(s.csrfProtect).Post("/links/{id}/edit", s.handleAdminEditPost)
```

- [ ] **Step 5: Run tests**

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/web/admin.go internal/web/admin_test.go internal/web/server.go internal/web/templates/admin_edit.html internal/shortener/service.go
git commit -m "feat(web): add admin edit-destination page"
```

---

### Task 20: Wire main.go — `migrate` and `set-password` subcommands

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Implement migrate + set-password**

Replace `main.go` with:

```go
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"golang.org/x/term"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "migrate":
		os.Exit(cmdMigrate(args))
	case "set-password":
		os.Exit(cmdSetPassword(args))
	case "serve":
		os.Exit(cmdServe(args))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ffsbz <serve|set-password|migrate> [flags]")
}

func openStore(dbPath string) (*store.Store, error) {
	if dbPath == "" {
		return nil, errors.New("--db is required")
	}
	return store.Open(dbPath)
}

func cmdMigrate(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbPath := fs.String("db", "ffsbz.db", "path to SQLite database")
	_ = fs.Parse(args)

	s, err := openStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}
	defer s.Close()
	fmt.Println("migrations applied")
	return 0
}

func cmdSetPassword(args []string) int {
	fs := flag.NewFlagSet("set-password", flag.ExitOnError)
	dbPath := fs.String("db", "ffsbz.db", "path to SQLite database")
	_ = fs.Parse(args)

	s, err := openStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "set-password:", err)
		return 1
	}
	defer s.Close()

	pw, err := readPassword("New admin password: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	confirm, err := readPassword("Confirm: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if pw != confirm {
		fmt.Fprintln(os.Stderr, "passwords do not match")
		return 1
	}
	if len(pw) < 8 {
		fmt.Fprintln(os.Stderr, "password too short (min 8)")
		return 1
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := s.SetAdminPasswordHash(context.Background(), hash); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("admin password set")
	return 0
}

// readPassword reads from the terminal without echo. Falls back to stdin (echo)
// for piped input.
func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		return string(b), err
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line, nil
}

func cmdServe(args []string) int {
	// Stub - implemented in next task.
	fmt.Fprintln(os.Stderr, "serve not implemented yet")
	return 1
}
```

- [ ] **Step 2: Add the terminal dependency**

Run: `go get golang.org/x/term`

- [ ] **Step 3: Manual smoke test**

```bash
go build -o ./ffsbz .
./ffsbz migrate --db=/tmp/ffsbz-smoke.db
echo -e "hunter2x9\nhunter2x9" | ./ffsbz set-password --db=/tmp/ffsbz-smoke.db
```

Expected: "migrations applied" then "admin password set".

Inspect: `sqlite3 /tmp/ffsbz-smoke.db "SELECT id, length(password_hash) FROM admin;"` — should print `1|60`.

- [ ] **Step 4: Commit**

```bash
git add main.go go.mod go.sum
git commit -m "feat(cli): add migrate and set-password subcommands"
```

---

### Task 21: Wire main.go — `serve` subcommand

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Implement serve**

Replace the body of `cmdServe` in `main.go`:

```go
import (
	// (add to existing imports)
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/shortener"
	"ffs.bz/internal/store"
	"ffs.bz/internal/web"
)

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	dbPath := fs.String("db", "ffsbz.db", "path to SQLite database")
	secureCookies := fs.Bool("secure-cookies", false, "set Secure flag on session cookie")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	s, err := openStore(*dbPath)
	if err != nil {
		slog.Error("open store", "err", err)
		return 1
	}
	defer s.Close()

	if _, err := s.GetAdminPasswordHash(context.Background()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			slog.Error("no admin password set; run `ffsbz set-password` first")
			return 1
		}
		slog.Error("read admin password", "err", err)
		return 1
	}

	sessions := auth.NewSessionManager(s, auth.SessionConfig{
		CookieName:    "ffsbz_session",
		TTL:           7 * 24 * time.Hour,
		SecureCookies: *secureCookies,
		LoginPath:     "/admin/login",
	})
	clicks := clicklog.New(s, clicklog.DefaultConfig())
	clicks.Start()

	svr := web.NewServer(web.Deps{
		Store:     s,
		Shortener: shortener.New(s),
		Sessions:  sessions,
		Clicks:    clicks,
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           svr.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", *addr)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			return 1
		}
	case sig := <-sigCh:
		slog.Info("shutting down", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		clicks.Shutdown(shutdownCtx)
	}
	return 0
}
```

Remove the old stub `cmdServe` body (the one printing "serve not implemented yet").

- [ ] **Step 2: Build and smoke-test**

```bash
go build -o ./ffsbz .
rm -f /tmp/ffsbz-smoke.db
./ffsbz migrate --db=/tmp/ffsbz-smoke.db
echo -e "hunter2x9\nhunter2x9" | ./ffsbz set-password --db=/tmp/ffsbz-smoke.db
./ffsbz serve --addr=:18080 --db=/tmp/ffsbz-smoke.db &
SERVER_PID=$!
sleep 0.5
curl -i http://localhost:18080/admin/login | head -5    # expect 200 with HTML
curl -i http://localhost:18080/nope | head -5            # expect 404 with HTML
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null
```

Expected: login page renders; unknown slug 404s; process shuts down cleanly.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat(cli): add serve subcommand with graceful shutdown"
```

---

### Task 22: End-to-end smoke test + README

**Files:**
- Create: `e2e_test.go` (at module root)
- Create: `README.md`

- [ ] **Step 1: Write an end-to-end test**

Create `e2e_test.go` at the module root:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/shortener"
	"ffs.bz/internal/store"
	"ffs.bz/internal/web"
)

func TestEndToEndCreateAndRedirect(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	hash, _ := auth.HashPassword("hunter2x9")
	_ = s.SetAdminPasswordHash(context.Background(), hash)

	mgr := auth.NewSessionManager(s, auth.SessionConfig{
		CookieName: "ffsbz_session", TTL: time.Hour, LoginPath: "/admin/login",
	})
	cl := clicklog.New(s, clicklog.Config{BufferSize: 16, FlushMaxBatch: 4, FlushInterval: 20 * time.Millisecond})
	cl.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cl.Shutdown(ctx)
	}()

	svr := web.NewServer(web.Deps{
		Store:     s,
		Shortener: shortener.New(s),
		Sessions:  mgr,
		Clicks:    cl,
	})

	// 1. Log in.
	form := url.Values{"password": []string{"hunter2x9"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	cookie := rw.Result().Cookies()[0]

	// 2. Create a link.
	sess, _ := s.GetSession(context.Background(), cookie.Value)
	form = url.Values{
		"csrf_token":  []string{sess.CSRFToken},
		"slug":        []string{"blog"},
		"destination": []string{"https://example.com"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rw = httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusSeeOther {
		t.Fatalf("create status %d", rw.Code)
	}

	// 3. Visit /blog -> 302 to example.com.
	req = httptest.NewRequest(http.MethodGet, "/blog", nil)
	req.Header.Set("Referer", "https://hn.example")
	req.Header.Set("User-Agent", "e2e-bot")
	rw = httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	if rw.Code != http.StatusFound {
		t.Errorf("redirect status %d", rw.Code)
	}
	if rw.Header().Get("Location") != "https://example.com" {
		t.Errorf("location %q", rw.Header().Get("Location"))
	}

	// 4. Wait for the click to land.
	link, _ := s.GetLinkBySlug(context.Background(), "blog")
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, _ := s.CountClicks(context.Background(), link.ID)
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("click not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 5. Detail page shows the click.
	req = httptest.NewRequest(http.MethodGet, "/admin/links/1", nil)
	req.AddCookie(cookie)
	rw = httptest.NewRecorder()
	svr.Router().ServeHTTP(rw, req)
	if !strings.Contains(rw.Body.String(), "e2e-bot") {
		t.Errorf("user-agent missing from detail page")
	}
}
```

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 3: Create README**

Create `README.md`:

```markdown
# ffs.bz

A small single-binary URL shortener written in Go.

## Build

    go build -o ffsbz .

## First run

    ./ffsbz migrate
    ./ffsbz set-password
    ./ffsbz serve --addr=:8080

Visit `http://localhost:8080/admin/login`.

## Flags

- `serve --addr=:8080 --db=ffsbz.db [--secure-cookies]`
- `set-password --db=ffsbz.db`
- `migrate --db=ffsbz.db`

Set `--secure-cookies` when serving behind TLS termination.
```

- [ ] **Step 4: Commit**

```bash
git add e2e_test.go README.md
git commit -m "test: add end-to-end smoke test and README"
```

---

## Self-Review Notes

Spec coverage:
- Schema (links/clicks/admin/sessions, pragmas, FK cascade): Task 2.
- Random + custom slug, validation, reserved words: Task 9.
- Async click logger with drop counter and graceful shutdown: Task 10.
- Session cookie auth, bcrypt, login/logout: Tasks 7, 11, 14.
- CSRF on state-changing requests: Tasks 15, 17, 18, 19.
- Admin features (list, new, detail, edit, delete): Tasks 16–19.
- CLI subcommands (serve, set-password, migrate): Tasks 20–21.
- Logging via `log/slog`: Task 21.
- Embedded templates/static via `embed.FS`: Task 12.
- End-to-end happy path: Task 22.

No placeholders or TBDs. Type names referenced after introduction (`store.Link`, `clicklog.Event`, `auth.SessionManager`, `web.Deps`) are consistent across tasks. The `embed` parent-directory caveat in Task 2 is the only place where you may need to restructure; the fallback is described inline.
