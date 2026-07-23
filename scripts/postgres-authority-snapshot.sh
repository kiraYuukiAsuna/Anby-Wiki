#!/bin/sh
# Emits stable counts and SHA256 hashes for authoritative PostgreSQL tables.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib/backup-common.sh"

require_command psql

DATABASE_NAME=${1:-${PGDATABASE:-}}
OUTPUT=${2:-}
[ -n "$DATABASE_NAME" ] || die "authority-snapshot: database name is required"

TABLES="
wiki_site
namespace
actor
page
page_alias
page_redirect
content_snapshot
revision
entity_type
entity
entity_label
entity_alias
property
claim
claim_source
page_entity_binding
external_resource
asset
asset_revision
source
source_version
source_chunk
citation
proposal
proposal_operation
review_task
merge_conflict
change_batch
role
actor_role
page_protection
"

SQL_FILE=$(mktemp "${TMPDIR:-/tmp}/anby-authority.XXXXXX")
RAW_FILE=$(mktemp "${TMPDIR:-/tmp}/anby-authority-raw.XXXXXX")
DATA_DIR=$(mktemp -d "${TMPDIR:-/tmp}/anby-authority-data.XXXXXX")
trap 'rm -f "$SQL_FILE" "$RAW_FILE"; rm -rf "$DATA_DIR"' EXIT HUP INT TERM

{
  echo "BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;"
  first=1
  for table in $TABLES; do
    : >"$DATA_DIR/$table"
    if [ "$first" -eq 1 ]; then
      first=0
    else
      echo "UNION ALL"
    fi
    printf "SELECT '%s' AS table_name, to_jsonb(row_data)::text AS row_json FROM %s AS row_data\n" \
      "$table" "$table"
  done
  echo "ORDER BY 1, 2;"
  echo "COMMIT;"
} >"$SQL_FILE"

tab=$(printf '\t')
psql -X --no-psqlrc --quiet --set ON_ERROR_STOP=1 --tuples-only --no-align \
  --field-separator="$tab" --dbname="$DATABASE_NAME" \
  --file="$SQL_FILE" >"$RAW_FILE"

while IFS="$tab" read -r table row_json; do
  [ -n "$table" ] || continue
  printf '%s\n' "$row_json" >>"$DATA_DIR/$table"
done <"$RAW_FILE"

emit_snapshot() {
  for table in $TABLES; do
    count=$(wc -l <"$DATA_DIR/$table" | tr -d '[:space:]')
    hash=$(sha256_file "$DATA_DIR/$table")
    printf '%s\t%s\t%s\n' "$table" "$count" "$hash"
  done
}

if [ -n "$OUTPUT" ]; then
  emit_snapshot >"$OUTPUT"
else
  emit_snapshot
fi
