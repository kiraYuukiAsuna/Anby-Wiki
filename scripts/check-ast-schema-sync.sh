#!/bin/sh
# check-ast-schema-sync.sh — 校验 Typed Block AST Schema 副本与 contracts/ 权威原文件字节一致：
#   - 权威：contracts/schemas/ast/v1/ast.schema.json
#   - 副本：backend/internal/ast/schema/ast.schema.json（go:embed 无法跨 module 引用 ../../contracts）
# 修改 Schema 时必须先改 contracts/ 原文件，再同步副本。用法: sh scripts/check-ast-schema-sync.sh
set -u

fail() {
  echo "check-ast-schema-sync: $*" >&2
  exit 1
}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ORIGINAL="$SCRIPT_DIR/../contracts/schemas/ast/v1/ast.schema.json"
COPY="$SCRIPT_DIR/../backend/internal/ast/schema/ast.schema.json"

[ -f "$ORIGINAL" ] || fail "权威 Schema 不存在: $ORIGINAL"
[ -f "$COPY" ] || fail "内嵌副本不存在: $COPY"

if ! cmp -s "$ORIGINAL" "$COPY"; then
  fail "副本与权威 Schema 不一致：请编辑 contracts/schemas/ast/v1/ast.schema.json 后，将内容同步到 backend/internal/ast/schema/ast.schema.json"
fi

echo "check-ast-schema-sync: 通过（副本与 contracts/ 权威 Schema 字节一致）"
