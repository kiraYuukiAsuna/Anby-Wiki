#!/bin/sh
# Restores objects only into a new drill bucket and verifies every object SHA256.
set -eu
umask 077
LC_ALL=C
export LC_ALL

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib/backup-common.sh"

require_drill_environment

S3_ENDPOINT=${S3_ENDPOINT:-}
S3_REGION=${S3_REGION:-us-east-1}
BACKUP_DIR=${BACKUP_DIR:-}
RESTORE_BUCKET=${RESTORE_BUCKET:-}
REPORT_FILE=${OBJECT_RESTORE_REPORT:-}
OBJECT_CLI=${OBJECT_CLI:-}
[ -n "$S3_ENDPOINT" ] || die "object-restore: S3_ENDPOINT is required"
[ -n "$BACKUP_DIR" ] || die "object-restore: BACKUP_DIR is required"
[ -n "$RESTORE_BUCKET" ] || die "object-restore: RESTORE_BUCKET is required"
[ -n "$REPORT_FILE" ] || die "object-restore: OBJECT_RESTORE_REPORT is required"
[ ! -e "$REPORT_FILE" ] || die "object-restore: report destination already exists"

case "$RESTORE_BUCKET" in
  *-restore-*)
    case "$RESTORE_BUCKET" in
      *[!a-z0-9.-]*)
        die "object-restore: target bucket contains invalid characters"
        ;;
    esac
    ;;
  *)
    die "object-restore: target bucket name must contain -restore-"
    ;;
esac

[ "${OBJECT_RESTORE_CONFIRM:-}" = "CREATE_NEW_BUCKET:$RESTORE_BUCKET" ] ||
  die "object-restore: OBJECT_RESTORE_CONFIRM must name the new target bucket"
[ -f "$BACKUP_DIR/manifest.tsv" ] &&
  [ -f "$BACKUP_DIR/manifest.tsv.sha256" ] &&
  [ -f "$BACKUP_DIR/metadata.env" ] ||
  die "object-restore: incomplete backup package"

source_bucket=$(sed -n 's/^source_bucket=//p' "$BACKUP_DIR/metadata.env")
backup_cli=$(sed -n 's/^cli=//p' "$BACKUP_DIR/metadata.env")
[ -n "$source_bucket" ] || die "object-restore: source bucket metadata missing"
[ "$source_bucket" != "$RESTORE_BUCKET" ] ||
  die "object-restore: source and target buckets must differ"
[ -n "$OBJECT_CLI" ] || OBJECT_CLI=$backup_cli

verify_checksum_file "$BACKUP_DIR/manifest.tsv.sha256"
verify_object_manifest "$BACKUP_DIR/objects" "$BACKUP_DIR/manifest.tsv"

VERIFY_DIR=$(mktemp -d "${TMPDIR:-/tmp}/anby-object-verify.XXXXXX")
MC_CONFIG_DIR=
created=0
completed=0
cleanup() {
  status=$?
  if [ "$created" -eq 1 ] &&
    { [ "$completed" -ne 1 ] || [ "${OBJECT_RESTORE_CLEANUP:-on-failure}" = "always" ]; }; then
    case "$OBJECT_CLI" in
      aws)
        aws --endpoint-url "$S3_ENDPOINT" s3 rm "s3://$RESTORE_BUCKET/" \
          --recursive --only-show-errors >/dev/null 2>&1 || true
        aws --endpoint-url "$S3_ENDPOINT" s3api delete-bucket \
          --bucket "$RESTORE_BUCKET" >/dev/null 2>&1 || true
        ;;
      mc)
        mc --config-dir "$MC_CONFIG_DIR" rb --force \
          "anby-restore/$RESTORE_BUCKET" >/dev/null 2>&1 || true
        ;;
    esac
  fi
  rm -rf "$VERIFY_DIR"
  [ -z "$MC_CONFIG_DIR" ] || rm -rf "$MC_CONFIG_DIR"
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

case "$OBJECT_CLI" in
  aws)
    require_command aws
    export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-${S3_ACCESS_KEY:-}}
    export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-${S3_SECRET_KEY:-}}
    export AWS_DEFAULT_REGION=$S3_REGION
    [ -n "$AWS_ACCESS_KEY_ID" ] && [ -n "$AWS_SECRET_ACCESS_KEY" ] ||
      die "object-restore: S3 credentials are required"
    if aws --endpoint-url "$S3_ENDPOINT" s3api head-bucket \
      --bucket "$RESTORE_BUCKET" >/dev/null 2>&1; then
      die "object-restore: target bucket already exists; refusing to overwrite"
    fi
    started=$(now_epoch)
    started_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    if [ "$S3_REGION" = "us-east-1" ]; then
      aws --endpoint-url "$S3_ENDPOINT" s3api create-bucket \
        --bucket "$RESTORE_BUCKET" >/dev/null
    else
      aws --endpoint-url "$S3_ENDPOINT" s3api create-bucket \
        --bucket "$RESTORE_BUCKET" \
        --create-bucket-configuration "LocationConstraint=$S3_REGION" >/dev/null
    fi
    created=1
    aws --endpoint-url "$S3_ENDPOINT" s3 sync "$BACKUP_DIR/objects/" \
      "s3://$RESTORE_BUCKET/" --only-show-errors --no-progress
    aws --endpoint-url "$S3_ENDPOINT" s3 sync "s3://$RESTORE_BUCKET/" \
      "$VERIFY_DIR/" --only-show-errors --no-progress
    ;;
  mc)
    require_command mc
    [ -n "${S3_ACCESS_KEY:-}" ] && [ -n "${S3_SECRET_KEY:-}" ] ||
      die "object-restore: S3 credentials are required"
    MC_CONFIG_DIR=$(mktemp -d "${TMPDIR:-/tmp}/anby-mc.XXXXXX")
    mc --config-dir "$MC_CONFIG_DIR" alias set anby-restore "$S3_ENDPOINT" \
      "$S3_ACCESS_KEY" "$S3_SECRET_KEY" --api S3v4 >/dev/null
    if mc --config-dir "$MC_CONFIG_DIR" stat \
      "anby-restore/$RESTORE_BUCKET" >/dev/null 2>&1; then
      die "object-restore: target bucket already exists; refusing to overwrite"
    fi
    started=$(now_epoch)
    started_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    mc --config-dir "$MC_CONFIG_DIR" mb "anby-restore/$RESTORE_BUCKET" >/dev/null
    created=1
    mc --config-dir "$MC_CONFIG_DIR" mirror --overwrite \
      "$BACKUP_DIR/objects/" "anby-restore/$RESTORE_BUCKET/" >/dev/null
    mc --config-dir "$MC_CONFIG_DIR" mirror --overwrite \
      "anby-restore/$RESTORE_BUCKET/" "$VERIFY_DIR/" >/dev/null
    ;;
  *)
    die "object-restore: OBJECT_CLI must be aws or mc"
    ;;
esac

verify_object_manifest "$VERIFY_DIR" "$BACKUP_DIR/manifest.tsv"
finished=$(now_epoch)
finished_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
duration=$(elapsed_seconds "$started" "$finished")
object_count=$(wc -l <"$BACKUP_DIR/manifest.tsv" | tr -d '[:space:]')
cat >"$REPORT_FILE" <<EOF
format=m7-t06-object-restore-v1
target_bucket=$RESTORE_BUCKET
started_at=$started_at
completed_at=$finished_at
measured_rto_seconds=$duration
object_count=$object_count
manifest_sha256=pass
restored_object_sha256=pass
cleanup=${OBJECT_RESTORE_CLEANUP:-on-failure}
EOF

completed=1
echo "object-restore: verification passed"
echo "object-restore: objects=$object_count rto_seconds=$duration"
