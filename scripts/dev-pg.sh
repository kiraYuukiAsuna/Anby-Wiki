#!/bin/sh
# 本地开发/测试用 PostgreSQL 实例管理（Homebrew postgresql@17，免 Docker）。
# 数据目录在 /tmp（重启即失，仅用于开发与测试），端口 55432 避免与系统 Postgres 冲突。
#
# 用法：
#   sh scripts/dev-pg.sh start   # 初始化（如需）并启动，创建 wiki 库并应用迁移
#   sh scripts/dev-pg.sh stop    # 停止
#   sh scripts/dev-pg.sh reset   # 删库重建并重新迁移
#   sh scripts/dev-pg.sh status  # 状态
#
# 连接串：postgres://wiki@127.0.0.1:55432/wiki?sslmode=disable

set -eu
LC_ALL=C
export LC_ALL

PG_BIN="${PG_BIN:-/opt/homebrew/opt/postgresql@17/bin}"
DATA_DIR="${PGDATA:-/tmp/anby-wiki-pgdata}"
PORT="${PGPORT:-55432}"
DB_NAME="wiki"
DB_USER="wiki"
LOG="$DATA_DIR/server.log"

if [ ! -x "$PG_BIN/initdb" ]; then
  echo "dev-pg: 未找到 $PG_BIN/initdb，请先 brew install postgresql@17 或设置 PG_BIN" >&2
  exit 1
fi

running() {
  "$PG_BIN/pg_ctl" -D "$DATA_DIR" status >/dev/null 2>&1
}

case "${1:-}" in
  start)
    if [ ! -d "$DATA_DIR" ]; then
      "$PG_BIN/initdb" -D "$DATA_DIR" -U "$DB_USER" --auth=trust --encoding=UTF-8 >/dev/null
      echo "port = $PORT" >> "$DATA_DIR/postgresql.conf"
      echo "listen_addresses = '127.0.0.1'" >> "$DATA_DIR/postgresql.conf"
    fi
    if ! running; then
      "$PG_BIN/pg_ctl" -D "$DATA_DIR" -l "$LOG" -w start >/dev/null
    fi
    if ! "$PG_BIN/psql" -h 127.0.0.1 -p "$PORT" -U "$DB_USER" -d postgres -tAc \
        "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1; then
      "$PG_BIN/createdb" -h 127.0.0.1 -p "$PORT" -U "$DB_USER" "$DB_NAME"
    fi
    (cd "$(dirname "$0")/../backend" && \
      DATABASE_URL="postgres://$DB_USER@127.0.0.1:$PORT/$DB_NAME?sslmode=disable" \
      REDIS_URL=redis://127.0.0.1:6379 S3_ENDPOINT=x S3_BUCKET=x S3_ACCESS_KEY=x S3_SECRET_KEY=x \
      go run ./cmd/migrate up)
    echo "dev-pg: 已就绪 postgres://$DB_USER@127.0.0.1:$PORT/$DB_NAME?sslmode=disable"
    ;;
  stop)
    if running; then "$PG_BIN/pg_ctl" -D "$DATA_DIR" -w stop >/dev/null; echo "dev-pg: 已停止"; else echo "dev-pg: 未运行"; fi
    ;;
  reset)
    if running; then
      "$PG_BIN/dropdb" -h 127.0.0.1 -p "$PORT" -U "$DB_USER" --if-exists "$DB_NAME"
      "$PG_BIN/createdb" -h 127.0.0.1 -p "$PORT" -U "$DB_USER" "$DB_NAME"
    else
      rm -rf "$DATA_DIR"
    fi
    "$0" start
    ;;
  status)
    if running; then echo "dev-pg: 运行中 port=$PORT data=$DATA_DIR"; else echo "dev-pg: 未运行"; fi
    ;;
  *)
    echo "用法: $0 {start|stop|reset|status}" >&2
    exit 1
    ;;
esac
