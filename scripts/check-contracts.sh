#!/bin/sh
# Verify that embedded Go schema copies match the authoritative contract files.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

check_copy() {
  name=$1
  original=$2
  embedded=$3

  [ -f "$original" ] || {
    echo "contracts: authoritative $name schema is missing: $original" >&2
    exit 1
  }
  [ -f "$embedded" ] || {
    echo "contracts: embedded $name schema is missing: $embedded" >&2
    exit 1
  }
  cmp -s "$original" "$embedded" || {
    echo "contracts: $name embedded schema differs from its authoritative file" >&2
    echo "  authoritative: $original" >&2
    echo "  embedded:      $embedded" >&2
    exit 1
  }
  echo "contracts: $name schema copy is in sync"
}

check_copy \
  "Typed Block AST v1" \
  "$ROOT/contracts/schemas/ast/v1/ast.schema.json" \
  "$ROOT/backend/internal/ast/schema/ast.schema.json"

check_copy \
  "ProposalOperation v1" \
  "$ROOT/contracts/schemas/proposal-operation/v1/operation.schema.json" \
  "$ROOT/backend/internal/governance/schema/operation.schema.json"

check_copy \
  "Extraction candidates v1" \
  "$ROOT/contracts/schemas/extraction/v1/candidates.schema.json" \
  "$ROOT/backend/internal/importer/schema/candidates.schema.json"

echo "contracts: all embedded schema copies are in sync"
