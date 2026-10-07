#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/production-common.sh"
load_production_domain
[ "$#" -eq 0 ] || { echo "Usage: $0" >&2; exit 64; }
[ "$(id -u)" -eq 0 ] || { echo 'Run certificate setup as root.' >&2; exit 1; }
[ -L /etc/nginx/sites-enabled/anby-wiki ] || { echo 'Install the bootstrap Nginx site first.' >&2; exit 1; }
if [ -n "$CERTBOT_ACCOUNT" ]; then
  certbot show_account --account "$CERTBOT_ACCOUNT" >/dev/null
  set -- --account "$CERTBOT_ACCOUNT"
else
  set -- --email "$CERTBOT_EMAIL"
fi
nginx -t
install -d -m 0755 /var/www/letsencrypt
certbot certonly --webroot --webroot-path /var/www/letsencrypt \
  --domain "$ANBY_DOMAIN" --cert-name "$ANBY_DOMAIN" "$@" \
  --agree-tos --non-interactive --keep-until-expiring
sh "$ROOT/scripts/install-nginx.sh" production
install -d -m 0755 /etc/letsencrypt/renewal-hooks/deploy
install -m 0755 "$ROOT/scripts/reload-nginx.sh" \
  /etc/letsencrypt/renewal-hooks/deploy/anby-wiki-reload-nginx
systemctl enable --now certbot.timer
echo "TLS and automatic renewal enabled for https://$ANBY_DOMAIN/."
