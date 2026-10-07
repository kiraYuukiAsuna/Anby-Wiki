#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/production-common.sh"
load_production_domain
curl --fail --silent --show-error --max-time 30 "https://$ANBY_DOMAIN/healthz"
printf '\n'
curl --fail --silent --show-error --max-time 30 "https://$ANBY_DOMAIN/readyz"
printf '\n'
homepage=$(curl --fail --silent --show-error --max-time 30 "https://$ANBY_DOMAIN/")
printf '%s' "$homepage" | grep -q 'Anby Wiki'
production_compose exec -T worker wget -q -O /dev/null http://127.0.0.1:9091/metrics
production_compose --profile tools run --rm -T --interactive=false doctor
echo "HTTPS, API readiness, homepage, Worker and Doctor verified for $ANBY_DOMAIN."
