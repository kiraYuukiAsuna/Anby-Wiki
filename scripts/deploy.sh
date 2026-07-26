#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$ROOT/infra/deploy/compose.production.yml"}
DEPLOY_ENV_FILE=${DEPLOY_ENV_FILE:-"$ROOT/infra/deploy/.env"}

fail() {
  echo "deploy: $*" >&2
  exit 1
}

check_env_file_permissions() {
  mode=$(
    stat -c '%a' "$DEPLOY_ENV_FILE" 2>/dev/null ||
      stat -f '%Lp' "$DEPLOY_ENV_FILE" 2>/dev/null ||
      printf 'unknown'
  )
  case "$mode" in
    *00) ;;
    unknown) fail "cannot determine DEPLOY_ENV_FILE permissions" ;;
    *) fail "DEPLOY_ENV_FILE must not be group/world accessible (mode $mode)" ;;
  esac
}

[ -r "$DEPLOY_ENV_FILE" ] || fail "environment file is not readable: $DEPLOY_ENV_FILE"
check_env_file_permissions
set -a
# Deployment env files must remain shell-compatible KEY=VALUE files.
. "$DEPLOY_ENV_FILE"
set +a

# Build provenance is derived from the checked-out source, not maintained by
# operators in the environment file.
VCS_REF=$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf 'unknown')
BUILD_DATE=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
export VCS_REF BUILD_DATE

compose() {
  docker compose --env-file "$DEPLOY_ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

require_uint() {
  name=$1
  eval "value=\${$name:-}"
  case "$value" in
    "" | *[!0-9]* | 0) fail "$name must be a positive integer" ;;
  esac
}

check_window() {
  require_uint MIGRATION_EXPECTED_VERSION
  require_uint SCHEMA_MIN_COMPATIBLE_VERSION
  require_uint SCHEMA_MAX_COMPATIBLE_VERSION
  [ "$SCHEMA_MIN_COMPATIBLE_VERSION" -le "$SCHEMA_MAX_COMPATIBLE_VERSION" ] ||
    fail "schema compatibility window is reversed"
  [ "$MIGRATION_EXPECTED_VERSION" -ge "$SCHEMA_MIN_COMPATIBLE_VERSION" ] &&
    [ "$MIGRATION_EXPECTED_VERSION" -le "$SCHEMA_MAX_COMPATIBLE_VERSION" ] ||
    fail "expected migration is outside the image compatibility window"
}

check_release_id() {
  release=${RELEASE_ID:-}
  case "$release" in
    "" | *[!a-zA-Z0-9._-]* | .* | -.* | _.*)
      fail "RELEASE_ID must be a Docker-tag-safe version identifier"
      ;;
  esac
  [ "${#release}" -le 128 ] || fail "RELEASE_ID must not exceed 128 characters"
}

check_production_config() {
  [ "${ENV:-}" = "production" ] || fail "ENV must be production"
  [ "${AUTH_DEV_HEADER_ENABLED:-false}" = "false" ] ||
    fail "AUTH_DEV_HEADER_ENABLED must be false in production"
  check_release_id
  check_sensitive_env
}

confirm_production() {
  check_production_config
  expected="DEPLOY:${RELEASE_ID:?set RELEASE_ID}"
  [ "${DEPLOY_CONFIRM:-}" = "$expected" ] ||
    fail "set DEPLOY_CONFIRM=$expected for this release"
}

build_local_images() {
  compose build api worker web migrate
}

require_local_images() {
  for image in \
    "anby-wiki-api:$RELEASE_ID" \
    "anby-wiki-worker:$RELEASE_ID" \
    "anby-wiki-web:$RELEASE_ID" \
    "anby-wiki-migrate:$RELEASE_ID"
  do
    docker image inspect "$image" >/dev/null 2>&1 ||
      fail "local rollback image is missing: $image"
  done
}

run_gate() {
  check_window
  compose --profile tools run --rm migrate wiki-migrate up
  compose --profile tools run --rm migrate wiki-migrate check \
    "$MIGRATION_EXPECTED_VERSION" \
    "$SCHEMA_MIN_COMPATIBLE_VERSION" \
    "$SCHEMA_MAX_COMPATIBLE_VERSION"
  compose --profile tools run --rm doctor
}

check_existing_schema() {
  check_window
  compose --profile tools run --rm migrate wiki-migrate check \
    "$MIGRATION_EXPECTED_VERSION" \
    "$SCHEMA_MIN_COMPATIBLE_VERSION" \
    "$SCHEMA_MAX_COMPATIBLE_VERSION"
  compose --profile tools run --rm doctor
}

# Production secrets live in DEPLOY_ENV_FILE. Refuse missing values before any
# container is started.
check_sensitive_env() {
  for name in POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD S3_ACCESS_KEY S3_SECRET_KEY
  do
    eval "value=\${$name:-}"
    [ -n "$value" ] || fail "$name must be set in DEPLOY_ENV_FILE"
  done
  for name in POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD; do
    eval "value=\${$name}"
    case "$value" in
      *[!a-zA-Z0-9._-]*)
        fail "$name must contain only letters, digits, dot, underscore, or hyphen"
        ;;
    esac
  done
  if [ "${AI_IMPORT_ENABLED:-false}" = "true" ]; then
    for name in AI_BASE_URL AI_API_KEY AI_MODEL; do
      eval "value=\${$name:-}"
      [ -n "$value" ] || fail "$name must be set when AI_IMPORT_ENABLED=true"
    done
  fi
}

# start_data_tier brings up PostgreSQL/Redis/MinIO and ensures the bucket
# exists. These are stateful, so they are started before the application and
# are never recreated as part of an application roll.
start_data_tier() {
  compose --profile tools run --rm storage-init
  compose up -d --wait postgres redis minio
  compose --profile tools run --rm minio-init
}

roll_services() {
  # Keep this order stable: readers/writers before async consumers, then edge.
  compose up -d --no-deps --wait api
  compose up -d --no-deps --wait worker
  compose up -d --no-deps --wait web
}

command=${1:-}
case "$command" in
  config)
    compose config --quiet
    ;;
  build)
    check_production_config
    compose config --quiet
    build_local_images
    ;;
  migrate)
    confirm_production
    compose build migrate
    start_data_tier
    run_gate
    ;;
  doctor)
    compose --profile tools run --rm doctor
    ;;
  deploy)
    confirm_production
    compose config --quiet
    build_local_images
    start_data_tier
    run_gate
    roll_services
    ;;
  rollback)
    confirm_production
    compose config --quiet
    require_local_images
    # Rollback never executes down migrations. The old image must explicitly
    # declare compatibility with the database version that is already live.
    check_existing_schema
    roll_services
    ;;
  *)
    fail "usage: deploy.sh <config|build|migrate|doctor|deploy|rollback>"
    ;;
esac
