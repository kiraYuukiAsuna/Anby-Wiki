#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DOCKERFILE="$ROOT/Dockerfile"
COMPOSE_FILE="$ROOT/infra/deploy/compose.production.yml"
EXAMPLE_ENV="$ROOT/infra/deploy/.env.example"

fail() {
  echo "deploy config: $*" >&2
  exit 1
}

/bin/sh -n \
  "$ROOT/infra/deploy/container-entrypoint.sh" \
  "$ROOT/scripts/deploy.sh" \
  "$ROOT/scripts/check-deploy-config.sh" \
  "$ROOT/scripts/tests/deploy-test.sh"

if command -v ruby >/dev/null 2>&1; then
  ruby -e "require 'yaml'; YAML.safe_load(File.read(ARGV.fetch(0)), aliases: true)" "$COMPOSE_FILE"
fi

for target in api worker web migrate; do
  grep -Eq "^FROM .* AS ${target}$" "$DOCKERFILE" ||
    fail "Dockerfile target missing: $target"
done

grep -q '^USER 10001:10001$' "$DOCKERFILE" ||
  fail "Go runtime must use numeric non-root user"
grep -q '^USER node$' "$DOCKERFILE" ||
  fail "Web runtime must use non-root node user"
[ "$(grep -c '<<: \*runtime-security' "$COMPOSE_FILE")" -eq 7 ] ||
  fail "every production service must inherit runtime security"
grep -q 'read_only: true' "$COMPOSE_FILE" || fail "read_only runtime policy missing"
grep -q 'no-new-privileges:true' "$COMPOSE_FILE" || fail "no-new-privileges policy missing"
grep -q 'cap_drop:' "$COMPOSE_FILE" || fail "capability drop policy missing"

latest=$(
  find "$ROOT/backend/migrations" -type f -name '*.up.sql' -exec basename {} \; |
    sed 's/_.*//' |
    sort |
    tail -n 1 |
    sed 's/^0*//'
)
[ -n "$latest" ] || latest=0
expected=$(sed -n 's/^MIGRATION_EXPECTED_VERSION=//p' "$EXAMPLE_ENV")
[ "$expected" = "$latest" ] ||
  fail "example migration target $expected does not match repository latest $latest"

/bin/sh "$ROOT/scripts/tests/deploy-test.sh"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "deploy config: static checks passed; BLOCKED real compose/image validation (Docker unavailable)"
  exit 0
fi

docker compose --env-file "$EXAMPLE_ENV" -f "$COMPOSE_FILE" config --quiet
echo "deploy config: static and docker compose validation passed"
