# CLAUDE.md

Notes for future sessions working in this repo. Read once before making changes.

## What this is

A single-binary URL shortener written in Go. SQLite storage, single-user
admin behind a session cookie, async batched click writer. Module path is
`url-shortener` (the project was originally called `ffs.bz`; the design spec
and implementation plan keep the old name as a historical record — that's
intentional, don't "fix" it).

## Package layout & responsibilities

```
main.go                    CLI: serve | migrate | set-password | version
migrations/                Embedded SQL + tiny package exposing embed.FS
internal/store/            *sql.DB wrapper; CRUD for links, clicks, admin, sessions
internal/auth/             bcrypt + SessionManager + RequireAuth middleware
internal/shortener/        Slug generation + validation + Service.CreateLink
internal/clicklog/         Async batched click writer (buffered chan + worker)
internal/web/              chi router, handlers, embedded HTML + CSS
```

Files have one responsibility each. Keep them that way.

## Non-obvious constraints

- **No CGO.** SQLite driver is `modernc.org/sqlite` (pure Go) so cross-compile
  in CI works without a C toolchain. Don't swap to `mattn/go-sqlite3`.
- **`go:embed` can't traverse parents.** That's why `migrations/` is its own
  package exposing `migrations.FS` — the store package imports it. Don't
  collapse it.
- **Async clicks may drop.** The buffered channel intentionally drops on
  overflow (logged once per minute) so the redirect path never blocks on the
  DB. Don't change to blocking enqueue.
- **CSRF on every state-changing admin request.** Every POST/PUT/DELETE/PATCH
  under `/admin/*` runs through `csrfProtect`. New mutating routes must be
  wrapped in `.With(s.csrfProtect)`. The form has to include
  `<input type="hidden" name="csrf_token" value="{{.CSRF}}">`.
- **Session cookie is `urlshortener_session`.** Path is `/admin`. `Secure`
  flag is gated by `--secure-cookies` (set in production behind TLS).

## Conventions

- **Conventional commits.** GoReleaser groups the changelog by `feat:` and
  `fix:` prefixes — other prefixes (`docs:`, `chore:`, `test:`, `refactor:`,
  `ci:`) are filtered out of release notes. Keep this contract.
- **TDD.** Write the failing test first, confirm it fails, then implement.
  Tests use a real SQLite database in `t.TempDir()` — no mocks. Handler tests
  drive the full chi router via `httptest`.
- **Errors at boundaries, not internals.** Internal helpers may panic on
  programmer error (e.g. `loadTemplates` panics in `NewServer`); handlers
  return HTTP errors; the store layer returns typed errors like
  `ErrNotFound`, `ErrSlugTaken`, `ErrSessionExpired`.
- **No `--base-url` flag.** The original spec mentions it but no feature
  consumes it. Don't add it back without a use case.

## Where to look

- **Design intent:** `docs/superpowers/specs/2026-05-12-ffsbz-url-shortener-design.md`
- **Implementation plan (with code for every step):**
  `docs/superpowers/plans/2026-05-12-ffsbz-url-shortener.md`
- **Lint config:** `.golangci.yml` — uses standard linters with
  errcheck exclusions for idiomatic `Close()`/`Rollback()`. If you add a real
  unchecked error, fix it; don't widen the exclusion list.

## CI / release

- `.github/workflows/ci.yml` runs build + `go vet` + `go test` + `go test -race` + `golangci-lint` on push to main and on PRs.
- `.github/workflows/release.yml` triggers on `v*.*.*` tags. GoReleaser
  cross-compiles for linux/darwin/windows × amd64/arm64 (no Windows arm64),
  injects `version`/`commit`/`date` via `-ldflags` into `main.go`.
- Dependabot bumps gomod and github-actions weekly, grouped into one PR each.

To cut a release: `git tag -a vX.Y.Z -m "..." && git push origin vX.Y.Z`.

## Production deployment

The service is meant to run behind a reverse proxy that terminates TLS:

```
ExecStart=/opt/url-shortener/url-shortener serve \
  --addr=127.0.0.1:8080 \
  --db=/opt/url-shortener/data/store.db \
  --secure-cookies
```

Caddy then reverse-proxies the whole domain to `127.0.0.1:8080`. The admin
UI shares the same vhost — there's no separate admin port.

## Things to avoid

- Adding fields to public types without checking handler/template references.
  Templates use `html/template` (auto-escaping is good) but typos fail at
  runtime, not compile time.
- Calling `clicks.Shutdown` twice manually — it's idempotent (`sync.Once`)
  but the lifecycle is owned by `cmdServe`.
- Long-running queries inside `RequireAuth` — it runs on every admin request.
  If you need to add periodic work there (e.g. expired-session cleanup),
  fire-and-forget in a goroutine.
