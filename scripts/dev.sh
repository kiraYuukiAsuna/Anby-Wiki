#!/bin/sh
# 本地开发启动脚本（无 Docker）。
#
# 只负责跑起本仓库的三个进程：Go API、Go Worker、Next.js Web。
# PostgreSQL / Redis / MinIO 视为已经存在的外部依赖，连接信息全部经
# .env 提供，本脚本不安装也不启动这些组件。
#
# 用法：
#   cp .env.example .env   # 首次：填入外部依赖连接串
#   sh scripts/dev.sh              # 迁移 + 启动 api/worker/web
#   sh scripts/dev.sh --no-migrate # 跳过迁移
#   sh scripts/dev.sh api worker   # 只启动部分进程（可选：api worker web）
#
# 退出：Ctrl-C 一次即停止全部子进程。

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-"$ROOT/.env"}
RUN_MIGRATE=1
WANT_API=0
WANT_WORKER=0
WANT_WEB=0
SELECTED=0

fail() {
  echo "dev: $*" >&2
  exit 1
}

for arg in "$@"; do
  case "$arg" in
    --no-migrate) RUN_MIGRATE=0 ;;
    api) WANT_API=1; SELECTED=1 ;;
    worker) WANT_WORKER=1; SELECTED=1 ;;
    web) WANT_WEB=1; SELECTED=1 ;;
    -h | --help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
    *) fail "unknown argument: $arg (expected --no-migrate | api | worker | web)" ;;
  esac
done
if [ "$SELECTED" -eq 0 ]; then
  WANT_API=1
  WANT_WORKER=1
  WANT_WEB=1
fi

# ---- 环境变量 ----

[ -r "$ENV_FILE" ] || fail "environment file not found: $ENV_FILE (copy .env.example to .env)"
# .env 必须保持 shell 兼容的 KEY=VALUE 形式。
set -a
. "$ENV_FILE"
set +a

# ---- 前置检查：缺什么就直接说清楚，不要留到运行时深层报错 ----

command -v go >/dev/null 2>&1 || fail "go is not installed or not on PATH"
if [ "$WANT_WEB" -eq 1 ]; then
  command -v npm >/dev/null 2>&1 || fail "npm is not installed or not on PATH"
fi

missing=""
for name in DATABASE_URL REDIS_URL S3_ENDPOINT S3_BUCKET S3_ACCESS_KEY S3_SECRET_KEY; do
  eval "value=\${$name:-}"
  [ -n "$value" ] || missing="$missing $name"
done
[ -z "$missing" ] || fail "missing required variables in $ENV_FILE:$missing"

# 依赖可达性探测。用 Go 完成，避免依赖 nc/bash 等未必存在的工具。
probe() {
  label=$1
  target=$2
  if ! probe_error=$(GO_PROBE_TARGET="$target" go run "$ROOT/scripts/dev-probe.go" 2>&1); then
    fail "$label unreachable ($probe_error; start it, or fix $ENV_FILE)"
  fi
}

echo "dev: checking external dependencies..."
probe PostgreSQL "$(printf '%s' "$DATABASE_URL")"
probe Redis "$(printf '%s' "$REDIS_URL")"
probe "Object storage" "$(printf '%s' "$S3_ENDPOINT")"
echo "dev: dependencies reachable"

# ---- 迁移 ----

if [ "$RUN_MIGRATE" -eq 1 ]; then
  echo "dev: applying migrations..."
  (cd "$ROOT/backend" && go run ./cmd/migrate up)
fi

# ---- 启动子进程 ----

pids=""

# 终止整个进程组，避免留下孤儿 go run / next dev。
cleanup() {
  status=${1:-0}
  trap - INT TERM EXIT
  echo ""
  echo "dev: shutting down..."
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  for pid in $pids; do
    wait "$pid" 2>/dev/null || true
  done
  exit "$status"
}
trap cleanup INT TERM

# 直接后台运行每个命令，使记录的 PID 就是可终止的进程，而不是日志管道。
start() {
  label=$1
  shift
  "$@" &
  pids="$pids $!"
  echo "dev: started $label (pid $!)"
}

if [ "$WANT_API" -eq 1 ]; then
  start api sh -c "cd '$ROOT/backend' && exec go run ./cmd/api"
fi
if [ "$WANT_WORKER" -eq 1 ]; then
  start worker sh -c "cd '$ROOT/backend' && exec go run ./cmd/worker"
fi
if [ "$WANT_WEB" -eq 1 ]; then
  start web sh -c "cd '$ROOT/apps/web' && exec npm run dev"
fi

echo "dev: ready. API on \${PORT:-8080}, Web on 3000. Ctrl-C to stop."
# 任一子进程退出即整体收敛，避免只剩半套服务在跑。
set +e
wait -n
child_status=$?
set -e
cleanup "$child_status"
