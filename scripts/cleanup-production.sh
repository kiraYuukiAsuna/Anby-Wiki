#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DEPLOY_ENV_FILE=${DEPLOY_ENV_FILE:-"$ROOT/.env"}

fail() {
  echo "production cleanup: $*" >&2
  exit 1
}

[ "$(id -u)" -eq 0 ] || fail "run host cleanup as root"
for command in docker python3; do
  command -v "$command" >/dev/null || fail "missing cleanup tool: $command"
done
[ -r "$DEPLOY_ENV_FILE" ] || fail "environment file is not readable: $DEPLOY_ENV_FILE"

release=$(python3 "$ROOT/scripts/production_environment.py" get "$DEPLOY_ENV_FILE" RELEASE_ID)
case "$release" in
  "" | *[!a-zA-Z0-9._-]*) fail "invalid RELEASE_ID" ;;
esac

for entry in \
  "anby-wiki-production-ai-kernel-1:anby-wiki-ai-kernel" \
  "anby-wiki-production-api-1:anby-wiki-api" \
  "anby-wiki-production-worker-1:anby-wiki-worker" \
  "anby-wiki-production-web-1:anby-wiki-web"
do
  container=${entry%%:*}
  repository=${entry#*:}
  image=$(docker inspect --format '{{.Config.Image}}' "$container" 2>/dev/null || true)
  [ "$image" = "$repository:$release" ] ||
    fail "$container is not running the accepted release $release"
done

rm -rf -- "$ROOT/Secret/deploy-backups" "$ROOT/data/deploy-backups"

project=anby-wiki-production
containers=$(
  for status in created exited dead; do
    docker container ls -aq \
      --filter "label=com.docker.compose.project=$project" \
      --filter "status=$status"
  done
)
if [ -n "$containers" ]; then
  docker container rm $containers >/dev/null
fi

for repository in \
  anby-wiki-ai-kernel \
  anby-wiki-api \
  anby-wiki-worker \
  anby-wiki-web \
  anby-wiki-cli \
  anby-wiki-migrate
do
  docker image ls "$repository" --format '{{.Repository}}:{{.Tag}}' |
    while IFS= read -r image; do
      [ -n "$image" ] || continue
      [ "$image" = "$repository:$release" ] && continue
      if ! docker image rm "$image" >/dev/null; then
        echo "production cleanup: could not remove $image" >&2
      fi
    done
done

docker builder prune --all --force

echo "Production cleanup completed for Anby Wiki $release."
