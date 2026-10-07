#!/bin/sh

load_production_domain() {
  DEPLOY_ENV_FILE=${DEPLOY_ENV_FILE:-"$ROOT/.env"}
  export DEPLOY_ENV_FILE
  # Only validated public settings are emitted, with shell-safe quoting.
  settings=$(python3 "$ROOT/scripts/production_environment.py" domain "$DEPLOY_ENV_FILE") || return 1
  eval "$settings"
  export ANBY_DOMAIN CERTBOT_ACCOUNT CERTBOT_EMAIL WEB_PORT
}

production_compose() {
  python3 "$ROOT/scripts/production_environment.py" exec "$DEPLOY_ENV_FILE" \
    docker compose --env-file "$DEPLOY_ENV_FILE" \
    -f "$ROOT/infra/deploy/compose.production.yml" "$@" </dev/null
}
