#!/bin/sh
# check-migrations.sh — 校验 backend/migrations 迁移文件规范：
#   - 命名 {seq}_{name}.up.sql / {seq}_{name}.down.sql，up/down 成对且 name 一致
#   - {seq} 为 6 位数字，全局唯一，从 000001 连续无跳号
#   - {name} 为小写蛇形（[a-z0-9_]，非空）
#   - 无游离文件（README.md 除外）
# 空目录（或仅 README.md）视为通过。用法: sh scripts/check-migrations.sh [迁移目录]
set -u

# 固定 C locale，保证 [a-z] 区间、sort 排序行为确定
LC_ALL=C
export LC_ALL

fail() {
  echo "check-migrations: $*" >&2
  exit 1
}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DIR=${1:-"$SCRIPT_DIR/../backend/migrations"}

[ -d "$DIR" ] || fail "目录不存在: $DIR"

tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT

up_list="$tmp/up"
down_list="$tmp/down"
: > "$up_list"
: > "$down_list"

for f in "$DIR"/*; do
  [ -e "$f" ] || continue # 空目录时 glob 不展开
  base=${f##*/}

  case "$base" in
    README.md) continue ;;
    ??????_*.up.sql | ??????_*.down.sql) ;;
    *) fail "游离文件或命名非法: ${base}（期望 {seq}_{name}.up.sql / {seq}_{name}.down.sql）" ;;
  esac

  seq=${base%%_*}
  rest=${base#*_}             # {name}.up.sql / {name}.down.sql
  name_dir=${rest%.sql}       # {name}.up / {name}.down
  direction=${name_dir##*.}   # up / down
  name=${name_dir%.*}         # {name}

  case "$seq" in
    *[!0-9]* | "") fail "seq 须为 6 位数字: $base" ;;
  esac
  [ "${#seq}" -eq 6 ] || fail "seq 须恰好 6 位数字: $base"

  case "$name" in
    "" | *[!a-z0-9_]* | _* | *_) fail "name 须为非空小写蛇形: $base" ;;
  esac

  case "$direction" in
    up) echo "$seq $name" >> "$up_list" ;;
    down) echo "$seq $name" >> "$down_list" ;;
    *) fail "后缀须为 .up.sql / .down.sql: $base" ;;
  esac
done

# up/down 成对且 name 一致：两个集合必须完全相同
sort "$up_list" > "$tmp/up.sorted"
sort "$down_list" > "$tmp/down.sorted"
if ! cmp -s "$tmp/up.sorted" "$tmp/down.sorted"; then
  echo "check-migrations: up/down 迁移不成对或 name 不一致（左 up，右 down）:" >&2
  diff "$tmp/up.sorted" "$tmp/down.sorted" >&2 || true
  exit 1
fi

# seq 从 000001 连续无跳号（up/down 成对已保证全局唯一）
expected=1
while read -r seq _name; do
  want=$(printf '%06d' "$expected")
  [ "$seq" = "$want" ] || fail "迁移序号不连续: 期望 ${want}，实际 ${seq}"
  expected=$((expected + 1))
done < "$tmp/up.sorted"

count=$((expected - 1))
echo "check-migrations: 通过（${count} 个迁移，目录 ${DIR}）"
