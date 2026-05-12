# ffs.bz URL shortener — design

**Status:** approved (design phase)
**Date:** 2026-05-12

## Goal

A small, self-contained URL shortener written in Go. Single binary, SQLite
storage, server-rendered admin UI behind a single-user password login. Tracks
click count, timestamp, referer, and user-agent per redirect.

## Non-goals

- Multiple admin users, roles, or invitations.
- Public sign-up or any non-admin UI besides the redirect itself.
- Analytics beyond per-link click history (no aggregations, charts, exports).
- Custom domains per link (the service runs under one domain).
- Rate limiting, abuse detection, or link expiry.

## Architecture

One Go process exposing two HTTP surfaces on the same chi router:

- **Public:** `GET /` (landing/404), `GET /{slug}` (redirect).
- **Admin:** `/admin/*`, gated by a session-cookie middleware.

Static assets (HTML templates, minimal CSS) and SQL migrations are embedded via
`embed.FS`, so the deployable artifact is a single binary plus a SQLite file.

### Module layout

```
ffs.bz/
├── main.go                       # flag parsing, subcommand dispatch
├── go.mod
├── internal/
│   ├── store/
│   │   ├── store.go              # *sql.DB wrapper, pragmas, migrations
│   │   ├── links.go              # Link CRUD
│   │   ├── clicks.go             # Click insert + queries
│   │   └── admin.go              # admin password + session rows
│   ├── shortener/
│   │   ├── code.go               # random base62 generator (collision-aware)
│   │   └── service.go            # CreateLink, ResolveLink, validation
│   ├── clicklog/
│   │   └── logger.go             # async buffered click writer
│   ├── auth/
│   │   ├── password.go           # bcrypt hash + verify
│   │   └── session.go            # session create/lookup/delete + middleware
│   └── web/
│       ├── server.go             # chi router wiring
│       ├── public.go             # GET /{slug}
│       ├── admin.go              # admin handlers
│       ├── login.go              # login/logout
│       ├── templates/            # embedded *.html
│       └── static/               # embedded CSS
└── migrations/
    └── 0001_init.sql             # embedded
```

### Dependencies

- `github.com/go-chi/chi/v5` — router.
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGO; clean cross-compile).
- `golang.org/x/crypto/bcrypt` — password hashing.
- stdlib: `database/sql`, `html/template`, `embed`, `net/http`, `log/slog`,
  `crypto/rand`.

## Data model

```sql
CREATE TABLE links (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  slug        TEXT    NOT NULL UNIQUE,
  destination TEXT    NOT NULL,
  created_at  INTEGER NOT NULL                 -- unix seconds
);
CREATE INDEX idx_links_created_at ON links(created_at DESC);

CREATE TABLE clicks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  ts         INTEGER NOT NULL,                 -- unix seconds
  referer    TEXT    NOT NULL DEFAULT '',
  user_agent TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_clicks_link_ts ON clicks(link_id, ts DESC);

CREATE TABLE admin (
  id            INTEGER PRIMARY KEY CHECK (id = 1),  -- single-row
  password_hash TEXT    NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  token       TEXT    PRIMARY KEY,             -- 32-byte hex
  csrf_token  TEXT    NOT NULL,                -- per-session, 32-byte hex
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL
);
```

Pragmas set at connection open: `journal_mode=WAL`, `synchronous=NORMAL`,
`foreign_keys=ON`, `busy_timeout=5000`. Click counts are derived
(`COUNT(*) FROM clicks WHERE link_id = ?`); admin list uses a
`LEFT JOIN ... GROUP BY`.

## Request flow

### Public redirect — `GET /{slug}`

1. Look up slug in `links` (single indexed query).
2. **Hit:** write `302 Found` with `Location: <destination>`. After the
   response is written, enqueue a click event on a buffered channel
   (non-blocking).
3. **Miss:** render a small 404 page.

### Async click logger

- One goroutine started at server boot, reading from a buffered
  `chan Event` (capacity 1024).
- **Overflow policy:** non-blocking send (`select { case ch <- ev: default: }`);
  on miss, increment a dropped-events counter logged once per minute. The
  redirect path never blocks on the DB.
- **Batching:** the worker collects up to 64 events or 200 ms of waiting,
  whichever comes first, then performs a single batched `INSERT ... VALUES
  (...), (...), ...` inside a transaction.
- **Graceful shutdown:** on SIGINT/SIGTERM, close the channel, drain remaining
  events, then exit.

### Admin auth

- `GET /admin/login` — renders form.
- `POST /admin/login` — bcrypt-compare password against
  `admin.password_hash`. On success:
  - Generate `token` (32 random bytes hex) and `csrf_token` (same).
  - Insert into `sessions` with `expires_at = now + 7d`.
  - `Set-Cookie: ffsbz_session=<token>; HttpOnly; SameSite=Lax; Path=/admin`
    (plus `Secure` when `--secure-cookies` is set).
  - Redirect to `/admin`.
- Middleware on `/admin/*` (except `login`): reads cookie, looks up session,
  checks `expires_at > now`. Missing/expired/invalid → 303 to `/admin/login`.
- `POST /admin/logout` — deletes session row, clears cookie.

### CSRF

State-changing admin requests (POST/DELETE) must carry a `csrf_token` form
field that matches the session row. Middleware on those routes rejects with
403 on mismatch. Templates render `<input type="hidden" name="csrf_token">`
into every form using a value pulled from the request context.

### Admin routes

| Method | Path                                  | Purpose                                   |
|--------|---------------------------------------|-------------------------------------------|
| GET    | `/admin`                              | List links + click counts                 |
| GET    | `/admin/new`                          | Create-link form                          |
| POST   | `/admin/new`                          | Create link                               |
| GET    | `/admin/links/{id}`                   | Link detail: total + last 100 clicks      |
| POST   | `/admin/links/{id}/edit`              | Update destination (slug immutable)       |
| POST   | `/admin/links/{id}/delete`            | Delete link + cascade clicks              |
| GET    | `/admin/login`, POST `/admin/login`   | Auth                                      |
| POST   | `/admin/logout`                       | Auth                                      |

### Slug rules

- **Random:** 6 chars from `a-z A-Z 0-9` minus the ambiguous set `0OIl`.
  Retry up to 5× on UNIQUE collision; if still failing, surface
  "couldn't allocate a slug, try again."
- **Custom:** must match `^[a-zA-Z0-9_-]{1,64}$`. Reserved words rejected:
  `admin`, `static`, `health`, plus the empty string.
- **Destination:** parsed with `net/url`, must be absolute with `http` or
  `https` scheme.

## CLI

Single binary with subcommands:

```
ffsbz serve           [--addr=:8080] [--db=ffsbz.db]
                      [--base-url=https://ffs.bz] [--secure-cookies]
ffsbz set-password    [--db=ffsbz.db]   # prompts for password (no echo)
ffsbz migrate         [--db=ffsbz.db]   # apply migrations and exit
```

`serve` runs migrations implicitly on startup. Defaults are usable for local
development without flags.

## Error handling

- **Public redirect:** quiet to the user (clean 404 / 500 page), loud in logs
  with slug and reason.
- **Admin forms:** validation errors re-render the form with the user's
  input preserved and a flash error banner.
- **Admin server errors:** generic "something went wrong" admin page; log
  includes request ID.
- **DB:** every query gets a request-scoped `context.Context`.
  `busy_timeout=5000` covers transient WAL contention. UNIQUE collisions on
  random slugs trigger retry; on custom slugs they surface as validation
  errors.
- **Panic recovery:** middleware logs panics and returns 500 — one bad
  handler does not kill the process.
- **Click overflow:** dropped events counted and logged once per minute, not
  per drop.

## Logging

`log/slog` with JSON output to stdout. One line per redirect (slug, status,
latency); one per admin action; periodic click-logger health line including
drop count.

## Testing strategy

- **Store:** fresh SQLite in tmpdir; exercise each repository method
  including UNIQUE collision, CASCADE delete, FK enforcement.
- **Service:** slug generation collision-retry, validation rules
  (reserved words, regex, URL scheme).
- **Handlers:** `httptest.NewRecorder` against the chi router with a real
  in-memory store. Covers: redirect happy path, 404, login success/failure,
  CSRF rejection, session expiry, create/edit/delete admin flows.
- **Click logger:** burst N events, drain, assert all written; verify
  overflow drop behavior.
- No browser-driven E2E tests; templates are asserted via response-body text
  checks.

Development uses TDD: each piece has a failing test before implementation.

## Out-of-scope (deferred)

- Bulk import/export of links.
- Per-link enable/disable toggles, link expiry.
- IP capture (privacy implications; not requested).
- Charts or time-series aggregations in admin.
- Multi-domain support.
