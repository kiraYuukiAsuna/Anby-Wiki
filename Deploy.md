# Deploy.md — 开发与部署指南

本文件描述两种运行方式：

- **开发（dev）**：不使用 Docker。直接跑起前端 `npm` 与后端 Go 进程；
  PostgreSQL / Redis / MinIO 视为外部依赖，连接信息经 `.env` 提供。
- **生产（production）**：用 Docker Compose 一并部署数据层与应用，
  不含反向代理。

> 早期阶段的取舍与安全影响见文末「早期阶段限制」，上线前必读。

---

## 1. 开发环境

### 1.1 前置依赖

宿主机需要：

| 组件 | 说明 |
|---|---|
| Go | 版本见 `backend/go.mod` 的 `go` 指令 |
| Node.js + npm | 版本见 `apps/web/package.json` |
| PostgreSQL | 权威数据 + Outbox，需可连接 |
| Redis | 缓存与限流计数，可丢弃 |
| S3 兼容对象存储 | 例如 MinIO |

这三个数据组件**不由脚本安装或启动**：用本机安装、远端实例或共享环境都可以，
脚本只负责启动本仓库的三个进程。

### 1.2 首次配置

```sh
cp .env.example .env     # 填入外部依赖的连接串
make bootstrap           # 安装 Go 与前端依赖
```

`.env` 至少需要以下变量，缺失时脚本会直接指出缺哪一个：

```sh
DATABASE_URL=postgres://wiki:wiki_dev_password@localhost:5432/wiki?sslmode=disable
REDIS_URL=redis://localhost:6379/0
S3_ENDPOINT=http://localhost:9000
S3_BUCKET=wiki-assets
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin_dev
```

本地是明文 HTTP，因此 `.env.example` 中已把 `SESSION_COOKIE_SECURE=false`，
否则浏览器不会回传会话 cookie。

### 1.3 启动

```sh
sh scripts/dev.sh        # 或 make dev
```

脚本按顺序执行：

1. 读取 `.env`；
2. 检查 `go` / `npm` 与必填变量；
3. TCP 探测 PostgreSQL / Redis / 对象存储，**不可达就直接失败并指明地址**，
   而不是等到运行期报深层错误；
4. 执行数据库迁移；
5. 并行启动 `api`、`worker`、`web`；任一进程退出时停止其余进程并返回该退出码。

访问 <http://localhost:3000>。前端把 `/api/*` 反代到 `API_BASE_URL`
（见 `apps/web/next.config.ts`），因此浏览器只需要一个端口。

常用变体：

```sh
sh scripts/dev.sh --no-migrate   # 跳过迁移
sh scripts/dev.sh api worker     # 只起后端
sh scripts/dev.sh web            # 只起前端
```

`Ctrl-C` 一次即停止全部子进程。

### 1.4 登录

早期阶段没有身份提供方，登录使用共享引导令牌：

1. 在 `.env` 设置 `AUTH_DEV_LOGIN_TOKEN=<任意值>`（`development` 下可留空则该端点不可用）；
2. 打开 <http://localhost:3000/login> 输入该令牌。

调试也可用 `AUTH_DEV_HEADER_ENABLED=true` + 请求头 `X-Actor-ID: <actor uuid>`，
该开关在 `production` 下被配置校验强制拒绝。

### 1.5 质量门禁

```sh
make lint test          # 静态检查 + 单元测试
make ci                 # 本地等价 CI
```

集成测试需要独立数据库：

```sh
TEST_DATABASE_URL=postgres://wiki@127.0.0.1:5432/wiki_test?sslmode=disable \
  make test-go-integration
```

未设置 `TEST_DATABASE_URL` 时集成测试会 skip。集成用例每例 Reset 全库，
必须 `-p 1` 串行；并行开发时请为每个 Agent 建独立库。

---

## 2. 生产部署

### 2.1 拓扑

```
                    ┌───────────────────────────── docker network: app ─┐
  外部访问 ─────────►│  web:3000 ──/api/*──► api:8080                    │
  (仅 web 发布端口)  │                         │                        │
                    │                         ├──► postgres:5432       │
                    │             worker ─────┤                        │
                    │                         ├──► redis:6379          │
                    │                         └──► minio:9000          │
                    └──────────────────────────────────────────────────┘
```

要点：

- **不含反向代理。** `web` 是唯一发布端口的服务，通过 Next.js rewrites 转发 `/api/*`。
- **本清单不终结 TLS。** 需要 HTTPS 请在外层（云 LB、Cloudflare、宿主机独立代理）终结。
- 数据层（postgres/redis/minio）由 Compose 自己拉起，使用命名卷持久化。
- 限流、安全响应头、身份头清洗全部在 Go API 内实现，不依赖代理。

### 2.2 构建并推送镜像

```sh
VERSION=2026.07.25.1
VCS_REF="$(git rev-parse HEAD)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

for target in api worker web migrate; do
  docker build \
    --target "$target" \
    --build-arg VERSION="$VERSION" \
    --build-arg VCS_REF="$VCS_REF" \
    --build-arg BUILD_DATE="$BUILD_DATE" \
    -t "registry.example.com/anby/anby-wiki-$target:$VERSION" .
  docker push "registry.example.com/anby/anby-wiki-$target:$VERSION"
done
```

不要部署可变的 `latest` tag。把每个镜像解析为 registry digest，
并以 `name@sha256:...` 形式写入环境文件——`deploy.sh` 会强制校验这一点。

### 2.3 准备机密

机密以文件形式挂载，不写进环境文件，避免出现在 `docker inspect` 与日志中。

```sh
sudo install -d -m 0700 /etc/anby-wiki/secrets
cd /etc/anby-wiki/secrets

# 数据库连接串。host 必须是 compose 服务名 postgres。
printf 'postgres://wiki:<PASSWORD>@postgres:5432/wiki?sslmode=disable' | sudo tee database_url >/dev/null
printf '<PASSWORD>'              | sudo tee postgres_password      >/dev/null
printf '<S3_ACCESS_KEY>'         | sudo tee s3_access_key          >/dev/null
printf '<S3_SECRET_KEY>'         | sudo tee s3_secret_key          >/dev/null
# 引导登录令牌。必须足够强：配置校验会拒绝弱值。
openssl rand -hex 32             | sudo tee auth_dev_login_token   >/dev/null

sudo chmod 0600 *
```

`database_url` 里的密码需与 `postgres_password` 一致，
`s3_access_key` / `s3_secret_key` 同时用作 MinIO 的 root 凭据与应用凭据。

### 2.4 准备环境文件

```sh
cp infra/deploy/.env.example /etc/anby-wiki/.env
sudo chmod 0600 /etc/anby-wiki/.env
```

必须修改的项：

| 变量 | 说明 |
|---|---|
| `API_IMAGE` `WORKER_IMAGE` `WEB_IMAGE` `MIGRATE_IMAGE` | `name@sha256:...` 不可变引用 |
| `RELEASE_ID` | 本次发布标识 |
| `SECRETS_DIR` | 机密目录，如 `/etc/anby-wiki/secrets` |
| `S3_BUCKET` | 对象存储桶名 |
| `TRUSTED_ORIGINS` | 用户实际访问的精确 origin，如 `https://wiki.example.com` |
| `SESSION_COOKIE_SECURE` | 外层提供 HTTPS 时设 `true`，否则 `false` |
| `WEB_BIND` `WEB_PORT` | 对外端口；默认 `127.0.0.1:3000` 只绑本机 |
| `TRUSTED_PROXY_IPS` | 仅在 API 直连对端可信且其传来的 `X-Forwarded-For` 已被清洗时填写；默认留空最安全 |

### 2.5 部署

```sh
export DEPLOY_ENV_FILE=/etc/anby-wiki/.env
export DEPLOY_CONFIRM="DEPLOY:$(sed -n 's/^RELEASE_ID=//p' "$DEPLOY_ENV_FILE")"

sh scripts/deploy.sh config     # 校验清单可解析
sh scripts/deploy.sh deploy     # 正式发布
```

`deploy` 的执行顺序：

1. 校验 `DEPLOY_ENV=production`、镜像为 digest、机密文件存在且非空；
2. 运行 `storage-init` 修正命名卷根目录属主；
3. 启动数据层 postgres / redis / minio 并等待健康；
4. 运行 `minio-init` 创建 bucket 并关闭匿名访问；
5. 执行迁移，再校验迁移版本落在镜像兼容窗口内；
6. 运行 `doctor` 自检；
7. 按 `api` → `worker` → `web` 顺序滚动替换。

任一步失败即中止，不会继续替换应用容器。

### 2.6 其他命令

```sh
sh scripts/deploy.sh migrate    # 只跑迁移与闸门
sh scripts/deploy.sh doctor     # 只跑自检
sh scripts/deploy.sh rollback   # 回滚到环境文件中的旧镜像；不执行 down 迁移
```

回滚**从不**执行 down 迁移：旧镜像必须显式声明与线上数据库版本兼容。
需要缩表时，先发布一个兼容新旧两版的中间版本。

### 2.7 备份

数据在命名卷 `pgdata` 与 `miniodata` 中，随 `docker compose down` 保留，
但 `down -v` 会删除。备份脚本见 `scripts/postgres-backup.sh`、
`scripts/object-storage-backup.sh`。

---

## 3. 早期阶段限制

以下取舍是当前阶段的显式决定，**公网暴露前必须处理**。
遗留项同时记录在 `Docs/OutstandingIssues.md`。

1. **登录不验证真实身份。**
   `POST /api/v1/auth/dev-login` 以共享令牌换取会话，
   任何持有该令牌的人都能以对应 Actor 身份操作，且无法区分具体是谁。
   仅适用于封闭的早期部署；接入真实身份提供方后应设 `AUTH_DEV_LOGIN_ENABLED=false`。

2. **默认无 TLS。**
   Compose 不终结 HTTPS。明文暴露时会话 cookie 与引导令牌会在网络上可见，
   请务必在外层提供 HTTPS，并同步设置 `SESSION_COOKIE_SECURE=true`。

3. **搜索只有 PostgreSQL FTS。**
   ADR-0012 的 10 万页面实测显示该实现吞吐仅约 1.1 req/s。
   数据量或并发上升后需要重新引入独立搜索引擎；
   `SearchAdapter` 接口已为此保留。

4. **限流为应用层 + Redis 固定窗口。**
   Redis 不可达时**放行**并记日志（可用性优先于严格限流），
   因此限流不能作为唯一的滥用防线。默认不信任 `X-Forwarded-For`，
   在当前 Web → API 拓扑下会按 Web 容器这个直连对端计数；只有上游已可靠清洗
   来源头、且 API 对端被列入 `TRUSTED_PROXY_IPS` 后，才会按最终客户端 IP 分桶。
