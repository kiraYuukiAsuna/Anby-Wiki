#!/bin/sh
set -u
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ORIGINAL="$SCRIPT_DIR/../contracts/schemas/extraction/v1/candidates.schema.json"
COPY="$SCRIPT_DIR/../backend/internal/importer/schema/candidates.schema.json"
if ! cmp -s "$ORIGINAL" "$COPY"; then
  echo "check-extraction-schema-sync: Extraction Schema 副本与权威文件不一致" >&2
  exit 1
fi
echo "check-extraction-schema-sync: 通过"
