#!/bin/sh
# Creates a self-verifying PostgreSQL backup package without logging credentials.
set -eu
umask 077
LC_ALL=C
export LC_ALL

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib/backup-common.sh"

require_command pg_dump
require_command pg_restore
require_command psql

SOURCE_DB_NAME=${SOURCE_DB_NAME:-${PGDATABASE:-}}
BACKUP_DIR=${BACKUP_DIR:-}
[ -n "$SOURCE_DB_NAME" ] || die "postgres-backup: SOURCE_DB_NAME or PGDATABASE is required"
[ -n "$BACKUP_DIR" ] || die "postgres-backup: BACKUP_DIR is required"
case "$SOURCE_DB_NAME" in
  *[!a-zA-Z0-9_]*)
    die "postgres-backup: source database name contains unsupported characters"
    ;;
esac
[ "${BACKUP_WRITE_QUIESCED:-}" = "YES" ] ||
  die "postgres-backup: set BACKUP_WRITE_QUIESCED=YES after application writes are quiesced"
[ ! -e "$BACKUP_DIR" ] || die "postgres-backup: destination already exists"

mkdir -p "$BACKUP_DIR"
started=$(now_epoch)
started_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

pg_dump --dbname="$SOURCE_DB_NAME" --format=custom --compress=9 \
  --no-owner --no-privileges --file="$BACKUP_DIR/database.dump"
pg_restore --list "$BACKUP_DIR/database.dump" >/dev/null

/bin/sh "$ROOT/scripts/postgres-authority-snapshot.sh" \
  "$SOURCE_DB_NAME" "$BACKUP_DIR/authority.tsv"

dump_sha=$(sha256_file "$BACKUP_DIR/database.dump")
authority_sha=$(sha256_file "$BACKUP_DIR/authority.tsv")
printf '%s  %s\n' "$dump_sha" "database.dump" >"$BACKUP_DIR/database.dump.sha256"
printf '%s  %s\n' "$authority_sha" "authority.tsv" >"$BACKUP_DIR/authority.tsv.sha256"

finished=$(now_epoch)
finished_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
duration=$(elapsed_seconds "$started" "$finished")
server_version=$(psql -X --no-psqlrc --quiet --tuples-only --no-align \
  --dbname="$SOURCE_DB_NAME" --command="SHOW server_version;" | tr -d '[:space:]')
schema_version=$(psql -X --no-psqlrc --quiet --tuples-only --no-align \
  --dbname="$SOURCE_DB_NAME" \
  --command="SELECT version::text || CASE WHEN dirty THEN '-dirty' ELSE '-clean' END FROM schema_migrations;" |
  tr -d '[:space:]')

cat >"$BACKUP_DIR/metadata.env" <<EOF
format=m7-t06-postgres-v1
source_database=$SOURCE_DB_NAME
postgres_version=$server_version
schema_version=$schema_version
started_at=$started_at
completed_at=$finished_at
backup_duration_seconds=$duration
measured_rpo_seconds=$duration
write_quiesced=true
EOF

echo "postgres-backup: package verified"
echo "postgres-backup: duration_seconds=$duration"
