#!/bin/sh
# Compare Go source with gofmt output while treating CRLF and LF as equivalent.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

find "$ROOT/backend" -type f -name '*.go' -print >"$TMP/files"
failed=0
while IFS= read -r file; do
  tr -d '\015' <"$file" >"$TMP/source"
  gofmt "$file" | tr -d '\015' >"$TMP/formatted"
  if ! cmp -s "$TMP/source" "$TMP/formatted"; then
    echo "gofmt required for: ${file#"$ROOT/"}" >&2
    failed=1
  fi
done <"$TMP/files"

[ "$failed" -eq 0 ] || exit 1
echo "go format: pass"
