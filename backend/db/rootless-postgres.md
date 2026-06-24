# Rootless Postgres for integration tests

The DB-gated integration tests in `internal/store` (`*_integration_test.go`) need a
real Postgres and `t.Skip` unless `TEST_DATABASE_URL` is set. The normal way to get
one is the documented workflow — `docker compose up -d` (see the repo root
`docker-compose.yml`).

This recipe is the fallback for a machine with **no Docker, no system Postgres, and
no sudo** (e.g. a locked-down CI box or sandbox). It installs nothing system-wide:
it downloads the Ubuntu Postgres `.deb` packages with `apt-get download` (which does
not need root), extracts them into `~/.cache/chillcheck-rootless-pg`, and runs a
private server on **port 5433**.

## Quick start

From the repo root, the Make targets wrap the script:

```bash
make test-integration   # start Postgres (downloads on first run) + run all backend tests
make pg-down            # stop the server when done (state is kept for reuse)
```

Or call the script directly:

```bash
backend/db/rootless-postgres.sh start          # download (first run), init, start, load schema
cd backend && TEST_DATABASE_URL="$(db/rootless-postgres.sh dsn)" go test ./...
backend/db/rootless-postgres.sh stop           # stop the server (state is kept for reuse)
```

`nuke` stops the server and deletes the cache/data dir entirely:

```bash
backend/db/rootless-postgres.sh nuke
```

The DSN is `postgres://chillcheck:chillcheck@localhost:5433/chillcheck?sslmode=disable`.

## What the script does (and the gotchas it handles)

It's the automation of the manual steps; the two non-obvious bits are worth knowing
if you ever do it by hand:

1. **Download without root.** `apt-get download postgresql-16 postgresql-client-16
   postgresql-common postgresql-client-common libpq5` fetches the `.deb`s into the
   current dir using the existing apt indexes — no `sudo`. Each is unpacked with
   `dpkg-deb -x <pkg>.deb <prefix>` (extract files only, no dpkg database, no
   maintainer scripts). Run with `LD_LIBRARY_PATH` and `PATH` pointed at the prefix.

2. **Socket path length.** Postgres requires its Unix-domain socket path be under
   108 bytes. A deep temp dir blows past that and the server fails to start with
   *"Unix-domain socket path … is too long"*. The tests connect over TCP
   (`127.0.0.1:5433`), so the socket is incidental — the script just puts it at a
   short path (`/tmp/ccpg.<uid>`) via `pg_ctl -o "-k <shortdir>"`.

`initdb` uses `--auth=trust` so the password in the DSN is accepted without real
auth — fine for a throwaway local test DB, not for anything exposed.

## Adapting it

Defaults assume Ubuntu/Debian + amd64 + Postgres 16. For another major version
change `PGVER` (and the package names); override the state dir with
`CHILLCHECK_PGHOME` or the socket dir with `CHILLCHECK_PGSOCK`.
