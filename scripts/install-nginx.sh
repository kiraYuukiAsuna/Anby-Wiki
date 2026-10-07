#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/production-common.sh"
load_production_domain

[ "$#" -ge 1 ] && [ "$#" -le 2 ] || { echo "Usage: $0 <bootstrap|production> [--render]" >&2; exit 64; }
[ "$#" -eq 1 ] || [ "$2" = --render ] || exit 64
case "$1" in
  bootstrap) template="$ROOT/infra/deploy/nginx/anby-wiki.bootstrap.conf" ;;
  production) template="$ROOT/infra/deploy/nginx/anby-wiki.conf" ;;
  *) echo 'Unknown Nginx mode.' >&2; exit 64 ;;
esac
render() {
  sed -e "s/__ANBY_DOMAIN__/$ANBY_DOMAIN/g" -e "s/__WEB_PORT__/$WEB_PORT/g" "$template"
}
if [ "${2:-}" = --render ]; then render; exit 0; fi
[ "$(id -u)" -eq 0 ] || { echo 'Run Nginx installation as root.' >&2; exit 1; }
if [ "$1" = production ]; then
  [ -s "/etc/letsencrypt/live/$ANBY_DOMAIN/fullchain.pem" ] &&
    [ -s "/etc/letsencrypt/live/$ANBY_DOMAIN/privkey.pem" ] ||
    { echo 'Certificate missing; install bootstrap and enable TLS first.' >&2; exit 1; }
fi
site=/etc/nginx/sites-available/anby-wiki
enabled=/etc/nginx/sites-enabled/anby-wiki
[ ! -L "$site" ] && { [ ! -e "$site" ] || [ -f "$site" ]; } || exit 1
[ ! -e "$enabled" ] || [ -L "$enabled" ] || exit 1
if [ -L "$enabled" ] && [ "$(readlink "$enabled")" != "$site" ]; then
  echo 'Existing Anby Wiki enabled site points elsewhere; refusing to replace it.' >&2
  exit 1
fi
work=$(mktemp -d)
had_site=false
had_enabled=false
changed=false
if [ -f "$site" ]; then cp -p "$site" "$work/previous"; had_site=true; fi
if [ -L "$enabled" ]; then had_enabled=true; fi
cleanup() {
  result=$?
  trap - EXIT
  if [ "$changed" = true ]; then
    if [ "$had_site" = true ]; then cp -p "$work/previous" "$site"; else rm -f "$site"; fi
    if [ "$had_enabled" = false ]; then rm -f "$enabled"; fi
    echo 'Previous Anby Wiki Nginx configuration restored.' >&2
  fi
  rm -f "$work/rendered" "$work/previous"
  rmdir "$work"
  exit "$result"
}
trap cleanup EXIT
render > "$work/rendered"
if grep -Eq '__[A-Z_]+__' "$work/rendered"; then echo 'Unresolved Nginx placeholder.' >&2; exit 1; fi
install -d -m 0755 /var/www/letsencrypt /etc/nginx/sites-available /etc/nginx/sites-enabled
changed=true
install -m 0644 "$work/rendered" "$site"
ln -sfn "$site" "$enabled"
nginx -t
systemctl reload nginx
changed=false
echo "Installed $1 Nginx configuration for $ANBY_DOMAIN."
