#!/bin/sh
# ProposalOperation v1 权威 Schema 与 Go 内嵌副本防漂移。
set -u

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ORIGINAL="$SCRIPT_DIR/../contracts/schemas/proposal-operation/v1/operation.schema.json"
COPY="$SCRIPT_DIR/../backend/internal/governance/schema/operation.schema.json"

if ! cmp -s "$ORIGINAL" "$COPY"; then
  echo "check-operation-schema-sync: ProposalOperation Schema 副本与权威文件不一致" >&2
  exit 1
fi
echo "check-operation-schema-sync: 通过"
