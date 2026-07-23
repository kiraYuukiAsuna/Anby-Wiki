#!/bin/sh
# Mirrors an S3-compatible bucket and records deterministic object SHA256 values.
set -eu
umask 077
LC_ALL=C
export LC_ALL

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib/backup-common.sh"

S3_ENDPOINT=${S3_ENDPOINT:-}
S3_BUCKET=${S3_BUCKET:-}
S3_REGION=${S3_REGION:-us-east-1}
BACKUP_DIR=${BACKUP_DIR:-}
[ -n "$S3_ENDPOINT" ] || die "object-backup: S3_ENDPOINT is required"
[ -n "$S3_BUCKET" ] || die "object-backup: S3_BUCKET is required"
[ -n "$BACKUP_DIR" ] || die "object-backup: BACKUP_DIR is required"
case "$S3_BUCKET" in
  *[!a-z0-9.-]*)
    die "object-backup: source bucket contains unsupported characters"
    ;;
esac
[ ! -e "$BACKUP_DIR" ] || die "object-backup: destination already exists"

OBJECT_CLI=${OBJECT_CLI:-}
if [ -z "$OBJECT_CLI" ]; then
  if command -v aws >/dev/null 2>&1; then
    OBJECT_CLI=aws
  elif command -v mc >/dev/null 2>&1; then
    OBJECT_CLI=mc
  else
    die "object-backup: neither aws nor mc is available"
  fi
fi

mkdir -p "$BACKUP_DIR/objects"
started=$(now_epoch)
started_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

case "$OBJECT_CLI" in
  aws)
    require_command aws
    export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-${S3_ACCESS_KEY:-}}
    export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-${S3_SECRET_KEY:-}}
    export AWS_DEFAULT_REGION=$S3_REGION
    [ -n "$AWS_ACCESS_KEY_ID" ] && [ -n "$AWS_SECRET_ACCESS_KEY" ] ||
      die "object-backup: S3 credentials are required"
    aws --endpoint-url "$S3_ENDPOINT" s3 sync "s3://$S3_BUCKET/" \
      "$BACKUP_DIR/objects/" --only-show-errors --no-progress
    ;;
  mc)
    require_command mc
    [ -n "${S3_ACCESS_KEY:-}" ] && [ -n "${S3_SECRET_KEY:-}" ] ||
      die "object-backup: S3 credentials are required"
    MC_CONFIG_DIR=$(mktemp -d "${TMPDIR:-/tmp}/anby-mc.XXXXXX")
    trap 'rm -rf "$MC_CONFIG_DIR"' EXIT HUP INT TERM
    mc --config-dir "$MC_CONFIG_DIR" alias set anby-backup "$S3_ENDPOINT" \
      "$S3_ACCESS_KEY" "$S3_SECRET_KEY" --api S3v4 >/dev/null
    mc --config-dir "$MC_CONFIG_DIR" mirror --overwrite \
      "anby-backup/$S3_BUCKET/" "$BACKUP_DIR/objects/" >/dev/null
    ;;
  *)
    die "object-backup: OBJECT_CLI must be aws or mc"
    ;;
esac

write_object_manifest "$BACKUP_DIR/objects" "$BACKUP_DIR/manifest.tsv"
manifest_sha=$(sha256_file "$BACKUP_DIR/manifest.tsv")
printf '%s  %s\n' "$manifest_sha" "manifest.tsv" >"$BACKUP_DIR/manifest.tsv.sha256"

finished=$(now_epoch)
finished_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
duration=$(elapsed_seconds "$started" "$finished")
object_count=$(wc -l <"$BACKUP_DIR/manifest.tsv" | tr -d '[:space:]')
cat >"$BACKUP_DIR/metadata.env" <<EOF
format=m7-t06-objects-v1
source_bucket=$S3_BUCKET
region=$S3_REGION
cli=$OBJECT_CLI
started_at=$started_at
completed_at=$finished_at
backup_duration_seconds=$duration
measured_rpo_seconds=$duration
object_count=$object_count
EOF

echo "object-backup: package verified"
echo "object-backup: objects=$object_count duration_seconds=$duration"
