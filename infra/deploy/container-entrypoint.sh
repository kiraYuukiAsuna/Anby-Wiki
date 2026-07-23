#!/bin/sh
set -eu

# Resolve supported *_FILE variables without exposing secret values in logs.
for name in \
  DATABASE_URL REDIS_URL S3_ACCESS_KEY S3_SECRET_KEY MEILI_API_KEY \
  OIDC_CLIENT_SECRET AI_API_KEY PERF_DATABASE_URL
do
  eval "file=\${${name}_FILE:-}"
  [ -n "$file" ] || continue
  [ -r "$file" ] || {
    echo "container-entrypoint: ${name}_FILE is not readable" >&2
    exit 1
  }
  value=$(cat "$file")
  [ -n "$value" ] || {
    echo "container-entrypoint: ${name}_FILE is empty" >&2
    exit 1
  }
  export "$name=$value"
  unset "${name}_FILE"
done

exec "$@"
