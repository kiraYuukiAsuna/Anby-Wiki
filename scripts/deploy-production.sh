#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
[ "$(id -u)" -eq 0 ] || { echo 'Run host deployment as root.' >&2; exit 1; }
case "${1:-}" in
  '') [ "$#" -eq 0 ] || exit 64; pull=true ;;
  --no-pull) [ "$#" -eq 1 ] || exit 64; pull=false ;;
  *) echo "Usage: $0 [--no-pull]" >&2; exit 64 ;;
esac
for command in git docker python3 flock nginx certbot curl openssl systemctl; do
  command -v "$command" >/dev/null || { echo "Missing deployment tool: $command" >&2; exit 1; }
done
umask 077
install -d -m 0700 "$ROOT/data" "$ROOT/Secret" "$ROOT/Secret/deploy-backups" "$ROOT/data/deploy-backups"
exec 9> "$ROOT/data/deploy.lock"
flock -n 9 || { echo 'Another Anby Wiki deployment is running.' >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo 'Commit or preserve checkout changes before deploying.' >&2; exit 1; }
if [ "$pull" = true ]; then git pull --ff-only </dev/null; fi
DEPLOY_ENV_FILE=${DEPLOY_ENV_FILE:-"$ROOT/.env"}
export DEPLOY_ENV_FILE
deployment="$(date -u '+%Y%m%dT%H%M%SZ')-$$"
private_backup="$ROOT/Secret/deploy-backups/$deployment"
data_backup="$ROOT/data/deploy-backups/$deployment"
install -d -m 0700 "$private_backup" "$data_backup"
previous=false
if [ -f "$DEPLOY_ENV_FILE" ]; then cp -p "$DEPLOY_ENV_FILE" "$private_backup/environment"; previous=true; fi
git rev-parse HEAD > "$private_backup/source-commit"
if [ -f /etc/nginx/sites-available/anby-wiki ]; then
  cp -p /etc/nginx/sites-available/anby-wiki "$private_backup/nginx.conf"
fi
cleanup() {
  result=$?
  trap - EXIT
  if [ "$result" -ne 0 ] && [ "$previous" = true ]; then
    cp -p "$private_backup/environment" "$DEPLOY_ENV_FILE"
    old_release=$(python3 "$ROOT/scripts/production_environment.py" get "$DEPLOY_ENV_FILE" RELEASE_ID)
    if DEPLOY_CONFIRM="DEPLOY:$old_release" sh "$ROOT/scripts/deploy.sh" rollback \
      > "$private_backup/rollback.log" 2>&1; then
      echo 'Previous application release restored; database migrations retained.' >&2
    else
      echo "Automatic application rollback failed; inspect $private_backup/rollback.log." >&2
    fi
  fi
  exit "$result"
}
trap cleanup EXIT
if [ "$previous" = true ] && [ "$(docker inspect --format '{{.State.Running}}' anby-wiki-production-postgres-1 2>/dev/null || true)" = true ]; then
  python3 "$ROOT/scripts/production_environment.py" exec "$DEPLOY_ENV_FILE" \
    sh -c 'docker exec anby-wiki-production-postgres-1 pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
    </dev/null > "$data_backup/postgres.sql"
fi
python3 "$ROOT/scripts/prepare-production.py" "$ROOT"
. "$ROOT/scripts/production-common.sh"
load_production_domain
bootstrap_pending=$(python3 -c 'import json,pathlib,sys; p=pathlib.Path(sys.argv[1]); print("true" if p.exists() and not json.loads(p.read_text()).get("initialized") else "false")' "$ROOT/Secret/bootstrap-admin.json")
if [ "$bootstrap_pending" = false ] && [ -s "/etc/letsencrypt/live/$ANBY_DOMAIN/fullchain.pem" ] &&
  openssl x509 -in "/etc/letsencrypt/live/$ANBY_DOMAIN/fullchain.pem" -checkend 0 -noout >/dev/null; then
  sh "$ROOT/scripts/install-nginx.sh" production
else
  sh "$ROOT/scripts/install-nginx.sh" bootstrap
fi
release=$(python3 "$ROOT/scripts/production_environment.py" get "$DEPLOY_ENV_FILE" RELEASE_ID)
DEPLOY_CONFIRM="DEPLOY:$release" sh "$ROOT/scripts/deploy.sh" deploy </dev/null
python3 "$ROOT/scripts/bootstrap-administrator.py" "$ROOT" "$DEPLOY_ENV_FILE"
# Refresh registration settings after the initial administrator has been verified.
production_compose up -d --no-deps --wait api worker
sh "$ROOT/scripts/enable-tls.sh" </dev/null
sh "$ROOT/scripts/smoke-production.sh" </dev/null
printf '%s\n' "$release" > "$ROOT/Secret/current-release"
echo "Deployed Anby Wiki $release at https://$ANBY_DOMAIN/ (loopback Web port $WEB_PORT)."
