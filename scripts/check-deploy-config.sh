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

for script in \
  "$ROOT/scripts/deploy.sh" \
  "$ROOT/scripts/check-deploy-config.sh" \
  "$ROOT/scripts/tests/deploy-test.sh"
do
  /bin/sh -n "$script"
done

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
# postgres redis minio api worker web migrate doctor = 8 (storage-init and
# minio-init are short-lived jobs with dedicated security policies).
[ "$(grep -c '<<: \*runtime-security' "$COMPOSE_FILE")" -eq 8 ] ||
  fail "every production service must inherit runtime security"
if grep -qE '^  (nginx|meilisearch):' "$COMPOSE_FILE"; then
  fail "production compose must not declare a reverse proxy or meilisearch service"
fi
grep -q '^  storage-init:' "$COMPOSE_FILE" ||
  fail "production compose must initialize named-volume ownership"
if grep -Eq '^[[:space:]]*secrets:' "$COMPOSE_FILE"; then
  fail "production compose must read secrets from DEPLOY_ENV_FILE, not Compose secrets"
fi
if grep -q '_FILE:' "$COMPOSE_FILE" || grep -q 'container-entrypoint' "$DOCKERFILE"; then
  fail "legacy file-based secret injection is still configured"
fi
for name in DATABASE_URL POSTGRES_PASSWORD S3_ACCESS_KEY S3_SECRET_KEY \
  AUTH_DEV_LOGIN_TOKEN
do
  grep -q "^${name}=" "$EXAMPLE_ENV" ||
    fail "deployment environment example is missing $name"
done
if grep -Eq '^(API|WORKER|WEB|MIGRATE)_IMAGE=' "$EXAMPLE_ENV" ||
  grep -Eq '\$\{(API|WORKER|WEB|MIGRATE)_IMAGE' "$COMPOSE_FILE"; then
  fail "application images must not depend on registry image variables"
fi
for target in api worker web migrate; do
  grep -Fq "image: anby-wiki-${target}:\${RELEASE_ID:?set RELEASE_ID}" "$COMPOSE_FILE" ||
    fail "local versioned image is missing for $target"
  grep -Eq "^[[:space:]]+target: ${target}$" "$COMPOSE_FILE" ||
    fail "Compose local build target is missing for $target"
done
[ "$(grep -c 'pull_policy: never' "$COMPOSE_FILE")" -eq 5 ] ||
  fail "all application and tool services must forbid registry pulls"
grep -Fq 'DATABASE_URL: "${DATABASE_URL:?set DATABASE_URL}"' "$COMPOSE_FILE" ||
  fail "application DATABASE_URL environment injection is missing"
grep -Fq 'POSTGRES_PASSWORD: "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}"' "$COMPOSE_FILE" ||
  fail "PostgreSQL password environment injection is missing"
grep -Fq 'S3_ACCESS_KEY: "${S3_ACCESS_KEY:?set S3_ACCESS_KEY}"' "$COMPOSE_FILE" ||
  fail "application S3 access key environment injection is missing"
grep -Fq 'S3_SECRET_KEY: "${S3_SECRET_KEY:?set S3_SECRET_KEY}"' "$COMPOSE_FILE" ||
  fail "application S3 secret environment injection is missing"
grep -Fq 'AUTH_DEV_LOGIN_TOKEN: "${AUTH_DEV_LOGIN_TOKEN:?set AUTH_DEV_LOGIN_TOKEN}"' "$COMPOSE_FILE" ||
  fail "bootstrap login token environment injection is missing"
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
