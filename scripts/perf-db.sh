#!/bin/sh
# Creates or replaces a local database that the performance command will accept.
set -eu

PG_BIN="${PG_BIN:-/opt/homebrew/opt/postgresql@17/bin}"
PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-55432}"
PGUSER="${PGUSER:-wiki}"
DB_NAME="${PERF_DB_NAME:-wiki_perf_m7t05}"

case "$DB_NAME" in
  wiki_perf_*) ;;
  *) echo "perf-db: database name must start with wiki_perf_" >&2; exit 1 ;;
esac

for command in psql createdb dropdb; do
  if [ ! -x "$PG_BIN/$command" ]; then
    echo "perf-db: missing $PG_BIN/$command" >&2
    exit 1
  fi
done

"$PG_BIN/dropdb" -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" --if-exists "$DB_NAME"
"$PG_BIN/createdb" -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" "$DB_NAME"
"$PG_BIN/psql" -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres \
  -v ON_ERROR_STOP=1 -c "COMMENT ON DATABASE \"$DB_NAME\" IS 'anby-wiki-performance-only'" >/dev/null

DATABASE_URL="postgres://$PGUSER@$PGHOST:$PGPORT/$DB_NAME?sslmode=disable"
(cd "$(dirname "$0")/../backend" && \
  DATABASE_URL="$DATABASE_URL" REDIS_URL=redis://127.0.0.1:6379 \
  S3_ENDPOINT=x S3_BUCKET=x S3_ACCESS_KEY=x S3_SECRET_KEY=x \
  go run ./cmd/migrate up)

echo "perf-db: ready"
echo "export PERF_DATABASE_URL='$DATABASE_URL'"
echo "export PERF_DATABASE_CONFIRM='ANBY_WIKI_PERF_ONLY'"
