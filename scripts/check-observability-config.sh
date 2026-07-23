#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

cd "$ROOT/backend"
go test ./internal/platform/observability -run TestObservabilityConfig -count=1

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "observability config: Docker unavailable; static YAML validation passed"
  exit 0
fi

cd "$ROOT"
docker compose -f infra/local/docker-compose.yml --profile observability config --quiet
docker run --rm --entrypoint /bin/promtool \
  -v "$ROOT/infra/local/observability:/etc/prometheus:ro" \
  prom/prometheus:v3.5.0 check config /etc/prometheus/prometheus.yml
docker run --rm \
  -v "$ROOT/infra/local/observability/otel-collector.yml:/etc/otelcol-contrib/config.yaml:ro" \
  otel/opentelemetry-collector-contrib:0.135.0 \
  validate --config=/etc/otelcol-contrib/config.yaml
