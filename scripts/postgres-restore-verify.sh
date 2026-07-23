#!/bin/sh
# Restores only into a new drill database, then verifies authority and projections.
set -eu
umask 077
LC_ALL=C
export LC_ALL

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib/backup-common.sh"

require_drill_environment
for command in psql createdb dropdb pg_restore go; do
  require_command "$command"
done

BACKUP_DIR=${BACKUP_DIR:-}
RESTORE_DB_NAME=${RESTORE_DB_NAME:-}
REPORT_DIR=${RESTORE_REPORT_DIR:-}
[ -n "$BACKUP_DIR" ] || die "postgres-restore: BACKUP_DIR is required"
[ -n "$RESTORE_DB_NAME" ] || die "postgres-restore: RESTORE_DB_NAME is required"
[ -n "$REPORT_DIR" ] || die "postgres-restore: RESTORE_REPORT_DIR is required"

case "$RESTORE_DB_NAME" in
  wiki_restore_*)
    suffix=${RESTORE_DB_NAME#wiki_restore_}
    case "$suffix" in
      ""|*[!a-z0-9_]*)
        die "postgres-restore: target database must match wiki_restore_[a-z0-9_]+"
        ;;
    esac
    ;;
  *)
    die "postgres-restore: target database must match wiki_restore_[a-z0-9_]+"
    ;;
esac

[ "${RESTORE_CONFIRM:-}" = "CREATE_NEW_DATABASE:$RESTORE_DB_NAME" ] ||
  die "postgres-restore: RESTORE_CONFIRM must name the new target database"
[ -f "$BACKUP_DIR/database.dump" ] &&
  [ -f "$BACKUP_DIR/database.dump.sha256" ] &&
  [ -f "$BACKUP_DIR/authority.tsv" ] &&
  [ -f "$BACKUP_DIR/authority.tsv.sha256" ] &&
  [ -f "$BACKUP_DIR/metadata.env" ] ||
  die "postgres-restore: incomplete backup package"
[ ! -e "$REPORT_DIR" ] || die "postgres-restore: report destination already exists"

source_database=$(sed -n 's/^source_database=//p' "$BACKUP_DIR/metadata.env")
[ -n "$source_database" ] || die "postgres-restore: source database metadata missing"
[ "$source_database" != "$RESTORE_DB_NAME" ] ||
  die "postgres-restore: source and target database names must differ"

verify_checksum_file "$BACKUP_DIR/database.dump.sha256"
verify_checksum_file "$BACKUP_DIR/authority.tsv.sha256"
pg_restore --list "$BACKUP_DIR/database.dump" >/dev/null

exists=$(psql -X --no-psqlrc --quiet --tuples-only --no-align \
  --dbname=postgres \
  --command="SELECT 1 FROM pg_database WHERE datname = '$RESTORE_DB_NAME';" |
  tr -d '[:space:]')
[ -z "$exists" ] || die "postgres-restore: target database already exists; refusing to overwrite"

created=0
completed=0
cleanup() {
  status=$?
  if [ "$created" -eq 1 ] &&
    { [ "$completed" -ne 1 ] || [ "${RESTORE_CLEANUP:-on-failure}" = "always" ]; }; then
    dropdb --if-exists "$RESTORE_DB_NAME" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$REPORT_DIR"
started=$(now_epoch)
started_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

createdb --template=template0 "$RESTORE_DB_NAME"
created=1
pg_restore --exit-on-error --no-owner --no-privileges \
  --dbname="$RESTORE_DB_NAME" "$BACKUP_DIR/database.dump"

/bin/sh "$ROOT/scripts/postgres-authority-snapshot.sh" \
  "$RESTORE_DB_NAME" "$REPORT_DIR/authority-before-rebuild.tsv"
cmp "$BACKUP_DIR/authority.tsv" "$REPORT_DIR/authority-before-rebuild.tsv" ||
  die "postgres-restore: authoritative counts or hashes differ after restore"

DATABASE_URL="host=${PGHOST:-127.0.0.1} port=${PGPORT:-5432} user=${PGUSER:-postgres} dbname=$RESTORE_DB_NAME sslmode=${PGSSLMODE:-prefer}"
export DATABASE_URL
export REDIS_URL=${REDIS_URL:-redis://127.0.0.1:6379}
export S3_ENDPOINT=${S3_ENDPOINT:-http://127.0.0.1:9000}
export S3_BUCKET=${S3_BUCKET:-restore-verification-unused}
export S3_ACCESS_KEY=${S3_ACCESS_KEY:-restore-verification-unused}
export S3_SECRET_KEY=${S3_SECRET_KEY:-restore-verification-unused}
export SEARCH_BACKEND=postgres
export ENV=development

doctor_before=0
(cd "$ROOT/backend" && go run ./cmd/doctor -format json) \
  >"$REPORT_DIR/doctor-before-rebuild.json" || doctor_before=$?
case "$doctor_before" in
  0|1) ;;
  *) die "postgres-restore: doctor failed to execute before rebuild" ;;
esac
dead_replay=not-requested
if [ "$doctor_before" -eq 1 ] && [ "${RESTORE_REPLAY_DEAD:-}" = "YES" ]; then
  (cd "$ROOT/backend" && go run ./cmd/worker -replay-dead) \
    >"$REPORT_DIR/replay-dead.log"
  dead_replay=pass
fi
(cd "$ROOT/backend" && go run ./cmd/worker -rebuild-all) \
  >"$REPORT_DIR/rebuild.log"
doctor_after=0
(cd "$ROOT/backend" && go run ./cmd/doctor -format json) \
  >"$REPORT_DIR/doctor-after-rebuild.json" || doctor_after=$?
case "$doctor_after" in
  0|1) ;;
  *) die "postgres-restore: doctor failed to execute after rebuild" ;;
esac

/bin/sh "$ROOT/scripts/postgres-authority-snapshot.sh" \
  "$RESTORE_DB_NAME" "$REPORT_DIR/authority-after-rebuild.tsv"
cmp "$BACKUP_DIR/authority.tsv" "$REPORT_DIR/authority-after-rebuild.tsv" ||
  die "postgres-restore: rebuild changed authoritative counts or hashes"

finished=$(now_epoch)
finished_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
duration=$(elapsed_seconds "$started" "$finished")
cat >"$REPORT_DIR/restore-metrics.env" <<EOF
format=m7-t06-restore-v1
target_database=$RESTORE_DB_NAME
started_at=$started_at
completed_at=$finished_at
measured_rto_seconds=$duration
projection_and_search_rebuild=pass
doctor_before_rebuild_exit=$doctor_before
doctor_after_rebuild_exit=$doctor_after
dead_outbox_replay=$dead_replay
authority_comparison=pass
cleanup=${RESTORE_CLEANUP:-on-failure}
EOF

completed=1
if [ "$doctor_after" -ne 0 ]; then
  echo "postgres-restore: verification completed with doctor issues" >&2
  echo "postgres-restore: rto_seconds=$duration" >&2
  exit 1
fi
echo "postgres-restore: verification passed"
echo "postgres-restore: rto_seconds=$duration"
