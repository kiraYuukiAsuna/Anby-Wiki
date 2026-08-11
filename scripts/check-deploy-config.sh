#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DOCKERFILE="$ROOT/Dockerfile"
COMPOSE_FILE="$ROOT/infra/deploy/compose.production.yml"
EXAMPLE_ENV="$ROOT/infra/deploy/.env.example"
DEV_EXAMPLE_ENV="$ROOT/.env.example"

fail() {
  echo "deploy config: $*" >&2
  exit 1
}

for script in \
  "$ROOT/scripts/deploy.sh" \
  "$ROOT/scripts/check-deploy-config.sh"
do
  /bin/sh -n "$script"
done

if command -v ruby >/dev/null 2>&1; then
  ruby -e "require 'yaml'; YAML.safe_load(File.read(ARGV.fetch(0)), aliases: true)" "$COMPOSE_FILE"
fi

for target in api worker web migrate ai-kernel; do
  grep -Eq "^FROM .* AS ${target}$" "$DOCKERFILE" ||
    fail "Dockerfile target missing: $target"
done

grep -q '^USER 10001:10001$' "$DOCKERFILE" ||
  fail "Go runtime must use numeric non-root user"
grep -q '^USER node$' "$DOCKERFILE" ||
  fail "Web runtime must use non-root node user"
# postgres redis minio meilisearch ai-kernel api worker web migrate doctor = 10
# (storage-init and minio-init are short-lived jobs with dedicated policies).
[ "$(grep -c '<<: \*runtime-security' "$COMPOSE_FILE")" -eq 10 ] ||
  fail "every production service must inherit runtime security"
if grep -qE '^  nginx:' "$COMPOSE_FILE"; then
  fail "production compose must not declare a reverse proxy service"
fi
grep -Eq '^    image: getmeili/meilisearch:v[0-9]+\.[0-9]+\.[0-9]+$' "$COMPOSE_FILE" ||
  fail "Meilisearch image must use a fixed semantic version"
grep -q '^  storage-init:' "$COMPOSE_FILE" ||
  fail "production compose must initialize named-volume ownership"
if grep -Eq '^[[:space:]]*secrets:' "$COMPOSE_FILE"; then
  fail "production compose must read secrets from DEPLOY_ENV_FILE, not Compose secrets"
fi
if grep -q '_FILE:' "$COMPOSE_FILE" || grep -q 'container-entrypoint' "$DOCKERFILE"; then
  fail "legacy file-based secret injection is still configured"
fi
for name in POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD S3_ACCESS_KEY S3_SECRET_KEY MEILI_MASTER_KEY AI_CONFIG_MASTER_KEY AI_KERNEL_INTERNAL_TOKEN
do
  grep -q "^${name}=" "$EXAMPLE_ENV" ||
    fail "deployment environment example is missing $name"
done
# 生产模板必须覆盖开发模板中的全部通用应用变量；只允许额外增加生产部署变量。
for name in $(sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' "$DEV_EXAMPLE_ENV"); do
  # Production Compose derives its internal PostgreSQL URL from the three
  # POSTGRES_* values, so operators do not fill the same password twice.
  case "$name" in
    DATABASE_URL | MEILI_URL | MEILI_API_KEY) continue ;;
  esac
  grep -q "^${name}=" "$EXAMPLE_ENV" ||
    fail "production environment example is missing common variable $name"
done
if grep -Eq '^(API|WORKER|WEB|MIGRATE|AI_KERNEL)_IMAGE=' "$EXAMPLE_ENV" ||
  grep -Eq '\$\{(API|WORKER|WEB|MIGRATE|AI_KERNEL)_IMAGE' "$COMPOSE_FILE"; then
  fail "application images must not depend on registry image variables"
fi
for target in api worker web migrate ai-kernel; do
  grep -Fq "image: anby-wiki-${target}:\${RELEASE_ID:?set RELEASE_ID}" "$COMPOSE_FILE" ||
    fail "local versioned image is missing for $target"
  grep -Eq "^[[:space:]]+target: ${target}$" "$COMPOSE_FILE" ||
    fail "Compose local build target is missing for $target"
done
[ "$(grep -c 'pull_policy: never' "$COMPOSE_FILE")" -eq 6 ] ||
  fail "all application and tool services must forbid registry pulls"
grep -Fq 'AI_CONFIG_MASTER_KEY: "${AI_CONFIG_MASTER_KEY:?set AI_CONFIG_MASTER_KEY}"' "$COMPOSE_FILE" ||
  fail "AI configuration master key injection is missing"
grep -Fq 'AI_KERNEL_INTERNAL_TOKEN: "${AI_KERNEL_INTERNAL_TOKEN:?set AI_KERNEL_INTERNAL_TOKEN}"' "$COMPOSE_FILE" ||
  fail "AI kernel internal token injection is missing"
grep -Fq 'DATABASE_URL: "postgres://${POSTGRES_USER:?set POSTGRES_USER}:${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB:?set POSTGRES_DB}?sslmode=disable"' "$COMPOSE_FILE" ||
  fail "application DATABASE_URL must be derived from POSTGRES_*"
grep -Fq 'POSTGRES_PASSWORD: "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}"' "$COMPOSE_FILE" ||
  fail "PostgreSQL password environment injection is missing"
grep -Fq 'S3_ACCESS_KEY: "${S3_ACCESS_KEY:?set S3_ACCESS_KEY}"' "$COMPOSE_FILE" ||
  fail "application S3 access key environment injection is missing"
grep -Fq 'S3_SECRET_KEY: "${S3_SECRET_KEY:?set S3_SECRET_KEY}"' "$COMPOSE_FILE" ||
  fail "application S3 secret environment injection is missing"
grep -Fq 'SEARCH_BACKEND: ${SEARCH_BACKEND:-meilisearch}' "$COMPOSE_FILE" ||
  fail "production search backend must default to Meilisearch"
grep -Fq 'MEILI_API_KEY: "${MEILI_MASTER_KEY:?set MEILI_MASTER_KEY}"' "$COMPOSE_FILE" ||
  fail "application Meilisearch key must be derived from MEILI_MASTER_KEY"
grep -Fq 'MEILI_MASTER_KEY: "${MEILI_MASTER_KEY:?set MEILI_MASTER_KEY}"' "$COMPOSE_FILE" ||
  fail "Meilisearch master key environment injection is missing"
grep -Fq 'AUTH_REGISTRATION_ENABLED: ${AUTH_REGISTRATION_ENABLED:-false}' "$COMPOSE_FILE" ||
  fail "account registration configuration is missing"
if grep -Eq '\$\{(POSTGRES|REDIS|MINIO|MINIO_CLIENT|MEILI|ALPINE)_IMAGE' "$COMPOSE_FILE" ||
  grep -Eq '^(POSTGRES|REDIS|MINIO|MINIO_CLIENT|MEILI|ALPINE)_IMAGE=' "$EXAMPLE_ENV"; then
  fail "third-party image versions must be fixed in Compose, not the environment file"
fi
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

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "deploy config: static checks passed; BLOCKED real compose/image validation (Docker unavailable)"
  exit 0
fi

docker compose --env-file "$EXAMPLE_ENV" -f "$COMPOSE_FILE" config --quiet
echo "deploy config: static and docker compose validation passed"
