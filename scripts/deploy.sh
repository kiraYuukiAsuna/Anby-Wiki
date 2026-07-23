#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$ROOT/infra/deploy/compose.production.yml"}
DEPLOY_ENV_FILE=${DEPLOY_ENV_FILE:-"$ROOT/infra/deploy/.env"}

fail() {
  echo "deploy: $*" >&2
  exit 1
}

[ -r "$DEPLOY_ENV_FILE" ] || fail "environment file is not readable: $DEPLOY_ENV_FILE"
set -a
# Deployment env files must remain shell-compatible KEY=VALUE files.
. "$DEPLOY_ENV_FILE"
set +a

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

require_digest_image() {
  name=$1
  eval "reference=\${$name:-}"
  case "$reference" in
    *@sha256:*) digest=${reference##*@sha256:} ;;
    *) fail "$name must be an immutable name@sha256:... reference" ;;
  esac
  case "$digest" in
    *[!0-9a-f]*) fail "$name has an invalid sha256 digest" ;;
  esac
  [ "${#digest}" -eq 64 ] || fail "$name sha256 digest must contain 64 hex characters"
}

confirm_production() {
  [ "${DEPLOY_ENV:-}" = "production" ] || fail "DEPLOY_ENV must be production"
  for image in API_IMAGE WORKER_IMAGE WEB_IMAGE MIGRATE_IMAGE NGINX_IMAGE; do
    require_digest_image "$image"
  done
  expected="DEPLOY:${RELEASE_ID:?set RELEASE_ID}"
  [ "${DEPLOY_CONFIRM:-}" = "$expected" ] ||
    fail "set DEPLOY_CONFIRM=$expected for this release"
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

roll_services() {
  # Keep this order stable: readers/writers before async consumers, then edge.
  compose up -d --no-deps --wait api
  compose up -d --no-deps --wait worker
  compose up -d --no-deps --wait web
  compose up -d --no-deps --wait nginx
}

command=${1:-}
case "$command" in
  config)
    compose config --quiet
    ;;
  migrate)
    confirm_production
    compose pull migrate
    run_gate
    ;;
  doctor)
    compose --profile tools run --rm doctor
    ;;
  deploy)
    confirm_production
    compose config --quiet
    compose pull api worker web migrate nginx
    run_gate
    roll_services
    ;;
  rollback)
    confirm_production
    compose config --quiet
    compose pull api worker web migrate nginx
    # Rollback never executes down migrations. The old image must explicitly
    # declare compatibility with the database version that is already live.
    check_existing_schema
    roll_services
    ;;
  seed)
    [ "${DEPLOY_ENV:-}" != "production" ] || fail "seed is forbidden in production"
    [ "${SEED_CONFIRM:-}" = "SEED_NON_PRODUCTION_PERF_DATABASE" ] ||
      fail "set SEED_CONFIRM=SEED_NON_PRODUCTION_PERF_DATABASE"
    compose --profile seed run --rm seed
    ;;
  *)
    fail "usage: deploy.sh <config|migrate|doctor|deploy|rollback|seed>"
    ;;
esac
