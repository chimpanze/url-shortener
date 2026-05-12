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
