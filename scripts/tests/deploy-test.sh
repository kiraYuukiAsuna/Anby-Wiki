#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/anby-deploy-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

fail() {
  echo "deploy-test: $*" >&2
  exit 1
}

mkdir -p "$TMP/bin"
cat >"$TMP/bin/docker" <<'FAKE_DOCKER'
#!/bin/sh
set -eu
: "${FAKE_DOCKER_LOG:?}"
printf '%s\n' "$*" >>"$FAKE_DOCKER_LOG"
case "$*" in
  *"${FAKE_FAIL_TOKEN:-__never_match__}"*) exit 42 ;;
esac
FAKE_DOCKER
chmod +x "$TMP/bin/docker"

cat >"$TMP/production.env" <<'ENV'
DEPLOY_ENV=production
DEPLOY_CONFIRM=DEPLOY:test
RELEASE_ID=test
API_IMAGE=registry.invalid/anby-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
WORKER_IMAGE=registry.invalid/anby-worker@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
WEB_IMAGE=registry.invalid/anby-web@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
MIGRATE_IMAGE=registry.invalid/anby-migrate@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
NGINX_IMAGE=registry.invalid/nginx@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
APP_NETWORK=test
MIGRATION_EXPECTED_VERSION=1
SCHEMA_MIN_COMPATIBLE_VERSION=1
SCHEMA_MAX_COMPATIBLE_VERSION=1
ENV

export PATH="$TMP/bin:$PATH"
export FAKE_DOCKER_LOG="$TMP/docker.log"
export DEPLOY_ENV_FILE="$TMP/production.env"

/bin/sh "$ROOT/scripts/deploy.sh" deploy

line_of() {
  awk -v pattern="$1" 'index($0, pattern) { print NR; exit }' "$FAKE_DOCKER_LOG"
}

migrate_line=$(line_of "wiki-migrate up")
check_line=$(line_of "wiki-migrate check 1 1 1")
doctor_line=$(line_of "run --rm doctor")
api_line=$(line_of "up -d --no-deps --wait api")
worker_line=$(line_of "up -d --no-deps --wait worker")
web_line=$(line_of "up -d --no-deps --wait web")
nginx_line=$(line_of "up -d --no-deps --wait nginx")

[ "$migrate_line" -lt "$check_line" ] &&
  [ "$check_line" -lt "$doctor_line" ] &&
  [ "$doctor_line" -lt "$api_line" ] &&
  [ "$api_line" -lt "$worker_line" ] &&
  [ "$worker_line" -lt "$web_line" ] &&
  [ "$web_line" -lt "$nginx_line" ] ||
  fail "rollout order is not migrate/check/doctor/api/worker/web/nginx"

: >"$FAKE_DOCKER_LOG"
export FAKE_FAIL_TOKEN="wiki-migrate up"
if /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "migration failure unexpectedly continued"
fi
if grep -q "up -d --no-deps --wait api" "$FAKE_DOCKER_LOG"; then
  fail "API rollout occurred after migration failure"
fi
unset FAKE_FAIL_TOKEN

sed 's#^API_IMAGE=.*#API_IMAGE=registry.invalid/anby-api:latest#' \
  "$TMP/production.env" >"$TMP/mutable.env"
if DEPLOY_ENV_FILE="$TMP/mutable.env" /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "mutable production image unexpectedly succeeded"
fi

if /bin/sh "$ROOT/scripts/deploy.sh" seed >/dev/null 2>&1; then
  fail "production seed unexpectedly succeeded"
fi

sed 's/^DEPLOY_ENV=production$/DEPLOY_ENV=staging/' "$TMP/production.env" >"$TMP/staging.env"
DEPLOY_ENV_FILE="$TMP/staging.env" \
  SEED_CONFIRM=SEED_NON_PRODUCTION_PERF_DATABASE \
  /bin/sh "$ROOT/scripts/deploy.sh" seed
grep -q -- "--profile seed run --rm seed" "$FAKE_DOCKER_LOG" ||
  fail "explicit non-production seed was not invoked"

echo "deploy-test: pass"
