#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
GITLEAKS_VERSION=${GITLEAKS_VERSION:-v8.28.0}
STAGING=$(mktemp -d)
LIST=$(mktemp)
trap 'rm -rf "$STAGING" "$LIST"' EXIT HUP INT TERM

# gitleaks dir does not honor .gitignore. Build an exact source snapshot from
# tracked and non-ignored untracked files so local 0600 deployment credentials,
# node_modules and build caches are never treated as repository content.
git -C "$ROOT" ls-files --cached --others --exclude-standard -z >"$LIST"
(
  cd "$ROOT"
  tar --null -T "$LIST" -cf -
) | (
  cd "$STAGING"
  tar -xf -
)

go run github.com/zricethezav/gitleaks/v8@"$GITLEAKS_VERSION" dir "$STAGING" \
  --config "$ROOT/.gitleaks.toml" --no-banner --redact
