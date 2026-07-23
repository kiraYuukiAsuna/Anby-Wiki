#!/bin/sh
# Runs the M9 background automation regression gate against one isolated test DB.
set -eu

if [ -z "${TEST_DATABASE_URL:-}" ]; then
  echo "m9-acceptance: TEST_DATABASE_URL is required" >&2
  exit 1
fi

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT/backend"

go test \
  ./internal/projection \
  ./internal/collection \
  ./internal/linkhealth \
  ./internal/knowledge \
  ./internal/governance \
  ./internal/doctor \
  ./cmd/worker \
  -count=1 -p 1
