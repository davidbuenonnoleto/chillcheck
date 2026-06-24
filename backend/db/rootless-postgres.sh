#!/usr/bin/env bash
# Rootless PostgreSQL for running the DB-gated integration tests when you have no
# Docker, no system Postgres, and no sudo. It downloads the Ubuntu Postgres .deb
# packages with `apt-get download` (no root needed), extracts them into a prefix,
# and runs a private instance on port 5433. Nothing is installed system-wide.
#
# Usage:
#   backend/db/rootless-postgres.sh start   # download (first run), init, start, load schema
#   backend/db/rootless-postgres.sh dsn     # print the TEST_DATABASE_URL to use
#   backend/db/rootless-postgres.sh stop    # stop the server
#   backend/db/rootless-postgres.sh nuke    # stop and delete everything
#
# Typical run:
#   backend/db/rootless-postgres.sh start
#   cd backend && TEST_DATABASE_URL="$(db/rootless-postgres.sh dsn)" go test ./...
#   backend/db/rootless-postgres.sh stop
#
# Assumes Ubuntu/Debian + amd64 + Postgres 16 in the apt sources. Adjust PGVER /
# the package list for other releases.
set -euo pipefail

PGVER=16
PORT=5433
# Keep state out of the repo and off any session-temp dir that gets cleaned up.
PGHOME="${CHILLCHECK_PGHOME:-$HOME/.cache/chillcheck-rootless-pg}"
PREFIX="$PGHOME/root"
DATA="$PGHOME/data"
# Postgres' unix socket path must be <108 bytes, so keep it short (not under $DATA).
SOCK="${CHILLCHECK_PGSOCK:-/tmp/ccpg.$(id -u)}"
BINDIR="$PREFIX/usr/lib/postgresql/$PGVER/bin"

export LD_LIBRARY_PATH="$PREFIX/usr/lib/x86_64-linux-gnu:$PREFIX/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export PATH="$BINDIR:$PATH"

DSN="postgres://chillcheck:chillcheck@localhost:$PORT/chillcheck?sslmode=disable"
SCHEMA="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/schema.sql"

fetch() {
  if [ -x "$BINDIR/postgres" ]; then return; fi
  echo ">> downloading Postgres $PGVER .debs (no root)…"
  mkdir -p "$PGHOME/debs" "$PREFIX"
  ( cd "$PGHOME/debs" && apt-get download \
      "postgresql-$PGVER" "postgresql-client-$PGVER" \
      postgresql-common postgresql-client-common libpq5 )
  for d in "$PGHOME"/debs/*.deb; do dpkg-deb -x "$d" "$PREFIX"; done
}

start() {
  fetch
  if [ ! -f "$DATA/PG_VERSION" ]; then
    echo ">> initdb…"
    initdb -D "$DATA" --auth=trust --username=postgres >/dev/null
  fi
  mkdir -p "$SOCK"
  if ! pg_ctl -D "$DATA" status >/dev/null 2>&1; then
    echo ">> starting server on port $PORT…"
    pg_ctl -D "$DATA" -l "$PGHOME/server.log" -w \
      -o "-p $PORT -k $SOCK -c listen_addresses=127.0.0.1" start
  fi
  # Create role/db once (idempotent).
  psql -h 127.0.0.1 -p "$PORT" -U postgres -d postgres -tAc \
    "SELECT 1 FROM pg_roles WHERE rolname='chillcheck'" | grep -q 1 || \
    psql -h 127.0.0.1 -p "$PORT" -U postgres -d postgres -v ON_ERROR_STOP=1 -c \
      "CREATE ROLE chillcheck LOGIN PASSWORD 'chillcheck' SUPERUSER" >/dev/null
  psql -h 127.0.0.1 -p "$PORT" -U postgres -d postgres -tAc \
    "SELECT 1 FROM pg_database WHERE datname='chillcheck'" | grep -q 1 || \
    psql -h 127.0.0.1 -p "$PORT" -U postgres -d postgres -v ON_ERROR_STOP=1 -c \
      "CREATE DATABASE chillcheck OWNER chillcheck" >/dev/null
  echo ">> loading schema…"
  psql "$DSN" -v ON_ERROR_STOP=1 -f "$SCHEMA" >/dev/null
  echo ">> ready. TEST_DATABASE_URL=$DSN"
}

case "${1:-start}" in
  start) start ;;
  dsn)   echo "$DSN" ;;
  stop)  pg_ctl -D "$DATA" -w stop ;;
  nuke)  pg_ctl -D "$DATA" -w stop 2>/dev/null || true; rm -rf "$PGHOME" "$SOCK"; echo "removed $PGHOME" ;;
  *)     echo "usage: $0 {start|dsn|stop|nuke}" >&2; exit 1 ;;
esac
