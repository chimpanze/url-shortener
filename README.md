# url-shortener

[![Go](https://img.shields.io/github/go-mod/go-version/chimpanze/url-shortener)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A small, self-contained URL shortener written in Go. Single binary, SQLite
storage, server-rendered admin UI behind a single-user password login.

## Features

- **One binary, one SQLite file.** Templates, static assets, and migrations
  are embedded; `go build` produces a self-contained executable.
- **Random or custom slugs.** 6-character base62 codes (ambiguous characters
  excluded) or admin-chosen slugs.
- **Click tracking.** Each redirect logs a timestamp, the `Referer` header,
  and the `User-Agent`. Writes are async and batched, so the redirect path
  never waits on the database.
- **Single-user admin** behind a bcrypt password and signed session cookie,
  with CSRF on every state-changing request.
- **Pure-Go SQLite driver** (`modernc.org/sqlite`) — cross-compile without
  CGO.

## Build

    go build -o url-shortener .

## First run

    ./url-shortener migrate
    ./url-shortener set-password
    ./url-shortener serve --addr=:8080

Then open <http://localhost:8080/admin/login> and sign in with the password
you just set.

## Commands

| Command                                  | What it does                                         |
|------------------------------------------|------------------------------------------------------|
| `url-shortener serve [flags]`            | Run the HTTP server.                                 |
| `url-shortener set-password [--db]`      | Prompt for a new admin password and store its hash.  |
| `url-shortener migrate [--db]`           | Apply any pending migrations and exit.               |

`serve` flags:

- `--addr=:8080` — listen address
- `--db=url-shortener.db` — SQLite file path
- `--secure-cookies` — set the `Secure` flag on the session cookie (use this
  when serving behind TLS termination)

## Architecture

```
main.go                    CLI dispatch (serve / set-password / migrate)
migrations/                Embedded SQL migrations
internal/store/            *sql.DB wrapper + CRUD for links, clicks, sessions, admin
internal/auth/             bcrypt + session manager + RequireAuth middleware
internal/shortener/        Slug generation, validation, link creation
internal/clicklog/         Async batched click writer (buffered chan + worker)
internal/web/              chi router, handlers, embedded templates & CSS
```

The redirect handler writes a `302 Found` and then non-blockingly enqueues a
click event on a buffered channel. A background goroutine drains the channel
in batches (up to 64 events or every 200 ms) and writes them in a single
transaction. If the buffer ever fills, events are dropped; the drop count is
logged once per minute.

## Development

    go test ./...
    go test -race ./...

Tests run against real SQLite databases in temp directories — no mocks.
Handler tests drive the full chi router end-to-end.

## License

MIT — see [LICENSE](LICENSE).
