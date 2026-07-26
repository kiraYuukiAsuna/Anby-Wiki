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

# Secret files must exist before deploy.sh will proceed.
mkdir -p "$TMP/secrets"
for name in database_url postgres_password s3_access_key s3_secret_key \
  auth_dev_login_token
do
  printf 'test-value\n' >"$TMP/secrets/$name"
  chmod 0600 "$TMP/secrets/$name"
done

cat >"$TMP/production.env" <<ENV
DEPLOY_ENV=production
DEPLOY_CONFIRM=DEPLOY:test
RELEASE_ID=test
API_IMAGE=registry.invalid/anby-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
WORKER_IMAGE=registry.invalid/anby-worker@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
WEB_IMAGE=registry.invalid/anby-web@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
MIGRATE_IMAGE=registry.invalid/anby-migrate@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
SECRETS_DIR=$TMP/secrets
S3_BUCKET=test-bucket
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

storage_line=$(line_of "run --rm storage-init")
data_line=$(line_of "up -d --wait postgres redis minio")
bucket_line=$(line_of "run --rm minio-init")
migrate_line=$(line_of "wiki-migrate up")
check_line=$(line_of "wiki-migrate check 1 1 1")
doctor_line=$(line_of "run --rm doctor")
api_line=$(line_of "up -d --no-deps --wait api")
worker_line=$(line_of "up -d --no-deps --wait worker")
web_line=$(line_of "up -d --no-deps --wait web")

[ -n "$storage_line" ] || fail "storage ownership init was not run"
[ -n "$data_line" ] || fail "data tier was not started"
[ "$storage_line" -lt "$data_line" ] &&
  [ "$data_line" -lt "$bucket_line" ] &&
  [ "$bucket_line" -lt "$migrate_line" ] &&
  [ "$migrate_line" -lt "$check_line" ] &&
  [ "$check_line" -lt "$doctor_line" ] &&
  [ "$doctor_line" -lt "$api_line" ] &&
  [ "$api_line" -lt "$worker_line" ] &&
  [ "$worker_line" -lt "$web_line" ] ||
  fail "rollout order is not storage/data/bucket/migrate/check/doctor/api/worker/web"

grep -q "up -d --no-deps --wait nginx" "$FAKE_DOCKER_LOG" &&
  fail "nginx must no longer be part of the rollout"

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

# A missing secret file must stop the deployment before any container runs.
sed "s#^SECRETS_DIR=.*#SECRETS_DIR=$TMP/absent#" \
  "$TMP/production.env" >"$TMP/nosecrets.env"
if DEPLOY_ENV_FILE="$TMP/nosecrets.env" /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly succeeded with a missing secrets directory"
fi

# An empty secret file is also rejected.
: >"$FAKE_DOCKER_LOG"
: >"$TMP/secrets/database_url"
if /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly succeeded with an empty secret file"
fi
printf 'test-value\n' >"$TMP/secrets/database_url"
chmod 0600 "$TMP/secrets/database_url"

# Group/world-readable secret files are rejected.
export FAKE_STAT_MODE=644
if /bin/sh "$ROOT/scripts/deploy.sh" deploy >/dev/null 2>&1; then
  fail "deploy unexpectedly succeeded with broad secret permissions"
fi
unset FAKE_STAT_MODE

echo "deploy-test: pass"
