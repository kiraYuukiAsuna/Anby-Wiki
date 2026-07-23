#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/anby-backup-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

fail() {
  echo "backup-restore-test: $1" >&2
  exit 1
}

mkdir -p "$TMP/bin" "$TMP/s3/source-bucket/nested"
printf 'alpha\n' >"$TMP/s3/source-bucket/a.txt"
printf 'beta\n' >"$TMP/s3/source-bucket/nested/file with spaces.txt"

cat >"$TMP/bin/aws" <<'FAKE_AWS'
#!/bin/sh
set -eu
: "${FAKE_S3_ROOT:?}"
if [ "${1:-}" = "--endpoint-url" ]; then
  shift 2
fi
service=$1
operation=$2
shift 2

bucket_path() {
  value=${1#s3://}
  bucket=${value%%/*}
  suffix=${value#"$bucket"}
  printf '%s/%s%s\n' "$FAKE_S3_ROOT" "$bucket" "$suffix"
}

case "$service:$operation" in
  s3:sync)
    source=$1
    destination=$2
    case "$source" in
      s3://*) source=$(bucket_path "$source") ;;
    esac
    case "$destination" in
      s3://*) destination=$(bucket_path "$destination") ;;
    esac
    mkdir -p "$destination"
    cp -R "$source/." "$destination/"
    ;;
  s3:rm)
    target=$(bucket_path "$1")
    rm -rf "$target"
    ;;
  s3api:head-bucket)
    [ "$1" = "--bucket" ]
    [ -d "$FAKE_S3_ROOT/$2" ]
    ;;
  s3api:create-bucket)
    [ "$1" = "--bucket" ]
    [ ! -e "$FAKE_S3_ROOT/$2" ]
    mkdir -p "$FAKE_S3_ROOT/$2"
    ;;
  s3api:delete-bucket)
    [ "$1" = "--bucket" ]
    rmdir "$FAKE_S3_ROOT/$2"
    ;;
  *)
    echo "fake aws: unsupported operation $service $operation" >&2
    exit 2
    ;;
esac
FAKE_AWS
chmod +x "$TMP/bin/aws"

export PATH="$TMP/bin:$PATH"
export FAKE_S3_ROOT="$TMP/s3"
export S3_ENDPOINT=http://fake.invalid
export S3_BUCKET=source-bucket
export S3_ACCESS_KEY=test-access-key
export S3_SECRET_KEY=test-secret-that-must-not-be-logged
export OBJECT_CLI=aws

BACKUP_DIR="$TMP/object-backup"
export BACKUP_DIR
/bin/sh "$ROOT/scripts/object-storage-backup.sh" >"$TMP/backup.log"
[ -f "$BACKUP_DIR/manifest.tsv.sha256" ] || fail "manifest checksum was not created"
grep -q "file with spaces.txt" "$BACKUP_DIR/manifest.tsv" ||
  fail "object with spaces is missing from manifest"

RESTORE_BUCKET=wiki-restore-test
OBJECT_RESTORE_CONFIRM="CREATE_NEW_BUCKET:$RESTORE_BUCKET"
OBJECT_RESTORE_REPORT="$TMP/object-restore.env"
OBJECT_RESTORE_CLEANUP=always
RESTORE_ENVIRONMENT=drill
export RESTORE_BUCKET OBJECT_RESTORE_CONFIRM OBJECT_RESTORE_REPORT OBJECT_RESTORE_CLEANUP
export RESTORE_ENVIRONMENT
/bin/sh "$ROOT/scripts/object-storage-restore-verify.sh" >"$TMP/restore.log"
[ ! -e "$TMP/s3/$RESTORE_BUCKET" ] || fail "drill bucket was not cleaned up"
grep -q '^restored_object_sha256=pass$' "$OBJECT_RESTORE_REPORT" ||
  fail "restore report did not record SHA256 success"

printf 'tampered\n' >>"$BACKUP_DIR/objects/a.txt"
OBJECT_RESTORE_REPORT="$TMP/tampered-restore.env"
export OBJECT_RESTORE_REPORT
if /bin/sh "$ROOT/scripts/object-storage-restore-verify.sh" >"$TMP/tampered.log" 2>&1; then
  fail "tampered object backup was accepted"
fi

if grep -R "$S3_SECRET_KEY" "$TMP/backup.log" "$TMP/restore.log" "$TMP/tampered.log"; then
  fail "secret appeared in script logs"
fi

mkdir -p "$TMP/safety-bin"
for command in psql createdb dropdb pg_restore go; do
  cat >"$TMP/safety-bin/$command" <<'FAKE_COMMAND'
#!/bin/sh
echo invoked >>"${SAFETY_LOG:?}"
exit 99
FAKE_COMMAND
  chmod +x "$TMP/safety-bin/$command"
done
SAFETY_LOG="$TMP/safety.log"
export SAFETY_LOG
if PATH="$TMP/safety-bin:$PATH" ENV=production \
  /bin/sh "$ROOT/scripts/postgres-restore-verify.sh" >"$TMP/production.log" 2>&1; then
  fail "production PostgreSQL restore was accepted"
fi
[ ! -e "$SAFETY_LOG" ] || fail "a PostgreSQL command ran before production refusal"

echo "backup-restore-test: PASS"
