# 本地基础设施（M0-T04）

Wiki 项目本地开发依赖的中间件，全部通过 Docker Compose 编排：

| 服务 | 镜像 | 宿主端口 | 说明 |
| --- | --- | --- | --- |
| postgres | `postgres:17-alpine` | 5432 | 权威数据 + Outbox（ADR-0003），库 `wiki` / 用户 `wiki` |
| redis | `redis:7-alpine` | 6379 | 仅缓存 / 限流 / 协调，不承载不可丢失数据 |
| meilisearch | `getmeili/meilisearch:v1.50.0` | 7700 | 内部 Beta 搜索索引；可从 Postgres staging 重建 |
| minio | `minio/minio:RELEASE.2025-09-07T16-13-09Z` | 9000（S3 API）/ 9001（Console） | S3 兼容对象存储（ADR-0004） |
| minio-init | `minio/mc` | — | 一次性 job：创建 bucket `wiki-assets` 并关闭匿名访问，完成后退出 |
| nginx | `nginx:1.27-alpine` | 8000 | 可选网关（profile `gateway`）：`/` → 宿主 3000（Next.js），`/api/` → 宿主 8080（Go API） |
| prometheus | `prom/prometheus:v3.5.0` | 9090 | 可选 `observability` profile，采集宿主 API/Worker |
| otel-collector | `otel/opentelemetry-collector-contrib:0.135.0` | 4317/4318 | 可选 `observability` profile，接收 OTLP Trace |

postgres / redis / meilisearch / minio 的数据分别持久化在命名卷
`pgdata` / `redisdata` / `meilidata` / `miniodata`。

## 前置条件

- macOS：安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/) 或 [OrbStack](https://orbstack.dev/)（推荐，更轻量），并确保 `docker compose version` 可用（Compose v2）。
- 首次使用复制环境变量样例：

  ```bash
  cp infra/local/.env.example infra/local/.env
  ```

  不复制也能跑，compose 文件内已提供全部开发默认值。

## 启动 / 停止 / 查看

使用仓库根目录的 Makefile（推荐）：

```bash
make infra-up    # docker compose -f infra/local/docker-compose.yml up -d --wait
make infra-down  # 停止并移除容器（保留数据卷）
make infra-ps    # 查看状态
```

`--wait` 会等待所有 healthcheck 通过，命令返回时 postgres / redis / meilisearch /
minio 已可用。

启动可选网关（反代宿主机上 `make dev-api` / `make dev-web` 启动的进程）：

```bash
docker compose -f infra/local/docker-compose.yml --profile gateway up -d
# 之后通过 http://localhost:8000 访问（/ -> Next.js，/api/ -> Go API）
```

启动可选观测组件：

```bash
make infra-observability-up
```

Prometheus 位于 <http://localhost:9090>，默认采集 API
<http://localhost:8080/metrics> 和 Worker <http://localhost:9091/>。应用设置
`OTEL_ENABLED=true`、`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317`、
`OTEL_EXPORTER_OTLP_INSECURE=true` 后向 Collector 导出 Trace；Collector 默认只使用
debug exporter。完整指标、告警与故障排查见 `Docs/observability.md`。

## 连接信息（开发默认值）

- PostgreSQL：`postgres://wiki:wiki_dev_password@localhost:5432/wiki?sslmode=disable`
- Redis：`redis://localhost:6379/0`
- Meilisearch：`http://localhost:7700`，开发 key `local-development-master-key`
- S3：endpoint `http://localhost:9000`，bucket `wiki-assets`，AK/SK `minioadmin` / `minioadmin_dev`，强制 path-style
- 网关：`http://localhost:8000`

来源导入 Worker 默认不领取任务。要接入支持 OpenAI-compatible
`/chat/completions` 与 JSON Schema structured output 的模型服务，给运行 Worker 的
进程设置 `AI_IMPORT_ENABLED=true`、`AI_BASE_URL`、`AI_API_KEY`、`AI_MODEL`；密钥只从
环境变量读取，配置与日志均不保存 Prompt、来源全文或凭据。

以上均可在 `infra/local/.env` 中覆盖（含端口，避免与本机已有服务冲突）。

## MinIO Console

浏览器打开 <http://localhost:9001>，用 `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`
（默认 `minioadmin` / `minioadmin_dev`）登录，可查看 `wiki-assets` bucket 及对象。
bucket 已关闭匿名访问，下载需凭据或 Presigned URL（应用侧 `PresignGet`，见 ADR-0004）。

## 数据库初始化说明

`postgres/init.sql` 挂载到 `/docker-entrypoint-initdb.d/`，仅在数据卷为空
（首次创建）时执行一次；脚本本身按幂等写法编写，可重复执行。它只创建
`pgcrypto` 扩展——**业务表（含 `outbox_event`）由 Go 迁移创建**：

```bash
make migrate-up
```

修改 `init.sql` 对已存在的数据卷不生效，需重置卷后重新初始化。

## 常见问题

**端口占用（5432 / 6379 / 7700 / 9000 / 9001 / 8000）**
在 `infra/local/.env` 中改 `POSTGRES_PORT` / `REDIS_PORT` / `MEILI_PORT` / `MINIO_API_PORT` /
`MINIO_CONSOLE_PORT` / `NGINX_PORT`，然后重新 `make infra-up`。注意同步修改
`DATABASE_URL` / `REDIS_URL` / `S3_ENDPOINT` 中的端口。

**重置全部数据（清空卷）**

```bash
docker compose -f infra/local/docker-compose.yml down -v
make infra-up
```

`down -v` 会删除 `pgdata` / `redisdata` / `meilidata` / `miniodata`，下次启动时重新执行
`init.sql` 和 minio-init。不可恢复，慎用。

**minio-init 重复执行**
minio-init 使用 `mc mb --ignore-existing`，重复运行（或重复 `up`）是幂等的，
bucket 已存在时直接跳过。

**Linux 上 nginx 无法解析 host.docker.internal**
compose 已加 `extra_hosts: host.docker.internal:host-gateway`，无需手工配置；
macOS / Windows 的 Docker Desktop 原生支持该域名。
