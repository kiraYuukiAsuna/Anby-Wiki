#!/bin/sh
set -eu

# 观测配置校验。
#
# 本地 Prometheus / OTel Collector 的 compose 定义已随 infra/local 移除：
# 开发环境不再用 Docker 起边车，指标由进程自身的 /metrics 暴露，
# OTLP 只在显式配置 OTEL_EXPORTER_OTLP_ENDPOINT 时启用。
# 因此这里只校验进程内观测配置的单元测试。

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

cd "$ROOT/backend"
go test ./internal/platform/observability -count=1

echo "observability config: in-process metrics and tracing configuration validated"
