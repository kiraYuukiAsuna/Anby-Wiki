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
cat >"$TMP/bin/stat" <<'FAKE_STAT'
#!/bin/sh
printf '%s\n' "${FAKE_STAT_MODE:-600}"
FAKE_STAT
chmod +x "$TMP/bin/stat"

cat >"$TMP/production.env" <<ENV
DEPLOY_ENV=production
DEPLOY_CONFIRM=DEPLOY:test
RELEASE_ID=test
DATABASE_URL=postgres://wiki:test-password@postgres:5432/wiki?sslmode=disable
POSTGRES_PASSWORD=test-password
S3_BUCKET=test-bucket
S3_ACCESS_KEY=test-access-key
S3_SECRET_KEY=test-secret-key
AUTH_DEV_LOGIN_TOKEN=test-bootstrap-login-token
TRUSTED_ORIGINS=http://wiki.invalid
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

build_line=$(line_of "build api worker web migrate")
storage_line=$(line_of "run --rm storage-init")
data_line=$(line_of "up -d --wait postgres redis minio")
bucket_line=$(line_of "run --rm minio-init")
migrate_line=$(line_of "wiki-migrate up")
check_line=$(line_of "wiki-migrate check 1 1 1")
doctor_line=$(line_of "run --rm doctor")
api_line=$(line_of "up -d --no-deps --wait api")
worker_line=$(line_of "up -d --no-deps --wait worker")
web_line=$(line_of "up -d --no-deps --wait web")

[ -n "$build_line" ] || fail "local application images were not built"
[ -n "$storage_line" ] || fail "storage ownership init was not run"
[ -n "$data_line" ] || fail "data tier was not started"
[ "$build_line" -lt "$storage_line" ] &&
  [ "$storage_line" -lt "$data_line" ] &&
  [ "$data_line" -lt "$bucket_line" ] &&
  [ "$bucket_line" -lt "$migrate_line" ] &&
  [ "$migrate_line" -lt "$check_line" ] &&
  [ "$check_line" -lt "$doctor_line" ] &&
  [ "$doctor_line" -lt "$api_line" ] &&
  [ "$api_line" -lt "$worker_line" ] &&
  [ "$worker_line" -lt "$web_line" ] ||
  fail "rollout order is not build/storage/data/bucket/migrate/check/doctor/api/worker/web"

if grep -Eq 'test-password|test-access-key|test-secret-key|test-bootstrap-login-token' \
  "$FAKE_DOCKER_LOG"; then
  fail "sensitive environment value appeared in deployment command log"
fi

grep -q "up -d --no-deps --wait nginx" "$FAKE_DOCKER_LOG" &&
  fail "nginx must no longer be part of the rollout"
grep -Eq '(^| )pull( |$)|(^| )push( |$)' "$FAKE_DOCKER_LOG" &&
  fail "commercial application images must not be pulled or pushed"

: >"$FAKE_DOCKER_LOG"
export FAKE_FAIL_TOKEN="wiki-migrate up"
if /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "migration failure unexpectedly continued"
fi
if grep -q "up -d --no-deps --wait api" "$FAKE_DOCKER_LOG"; then
  fail "API rollout occurred after migration failure"
fi
unset FAKE_FAIL_TOKEN

sed 's#^RELEASE_ID=.*#RELEASE_ID=invalid/release#' \
  "$TMP/production.env" >"$TMP/invalid-release.env"
if DEPLOY_ENV_FILE="$TMP/invalid-release.env" /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "invalid local release tag unexpectedly succeeded"
fi

# A missing sensitive environment value must stop deployment before containers run.
grep -v '^DATABASE_URL=' "$TMP/production.env" >"$TMP/missing-secret.env"
if DEPLOY_ENV_FILE="$TMP/missing-secret.env" /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly succeeded with a missing DATABASE_URL"
fi

# An empty sensitive environment value is also rejected.
: >"$FAKE_DOCKER_LOG"
sed 's#^DATABASE_URL=.*#DATABASE_URL=#' \
  "$TMP/production.env" >"$TMP/empty-secret.env"
if DEPLOY_ENV_FILE="$TMP/empty-secret.env" /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly succeeded with an empty DATABASE_URL"
fi

# Enabling AI import without its sensitive configuration is rejected before rollout.
printf '%s\n' 'AI_IMPORT_ENABLED=true' >>"$TMP/ai-missing.env"
grep -v '^AI_IMPORT_ENABLED=' "$TMP/production.env" >>"$TMP/ai-missing.env"
if DEPLOY_ENV_FILE="$TMP/ai-missing.env" /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly accepted incomplete AI import configuration"
fi

# A group/world-readable deployment environment file is rejected.
export FAKE_STAT_MODE=644
if /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly succeeded with broad environment-file permissions"
fi
unset FAKE_STAT_MODE

echo "deploy-test: pass"
