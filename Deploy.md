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
| --- | --- |
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

打开 <http://localhost:3000/register> 创建本地账号。首个注册账号自动成为站点
管理员，后续账号默认为编辑者；已有账号从 <http://localhost:3000/login>
使用用户名或邮箱和密码登录。建立所需账号后可设置
`AUTH_REGISTRATION_ENABLED=false` 关闭公开注册。

调试也可用 `AUTH_DEV_HEADER_ENABLED=true` + 请求头 `X-Actor-ID: <actor uuid>`，
该开关在 `production` 下被配置校验强制拒绝。

### 1.5 质量门禁

```sh
make check              # 提交前质量门禁
make ci                 # check + 生成物漂移 + 安全扫描
```

上述门禁执行格式化、静态分析、类型检查、构建、契约漂移、迁移规范、部署静态检查
和安全扫描；Go 单元测试及需要隔离全栈的 API/协作 E2E 另按下文执行。

---

## 2. 生产部署

### 2.1 拓扑

```text
                    ┌───────────────────────────── docker network: app ─┐
  外部访问 ─────────►│  web:3000 ──/api/*──► api:8080                    │
  (仅 web 发布端口)  │                         │                        │
                    │                         ├──► postgres:5432       │
                    │             worker ─────┤                        │
                    │                         ├──► redis:6379          │
                    │                         ├──► minio:9000          │
                    │                         └──► meilisearch:7700    │
                    └──────────────────────────────────────────────────┘
```

要点：

- **不含反向代理。** `web` 是唯一发布端口的服务，通过 Next.js rewrites 转发 `/api/*`。
- **本清单不终结 TLS。** 需要 HTTPS 请在外层（云 LB、Cloudflare、宿主机独立代理）终结。
- 数据与搜索层（postgres/redis/minio/meilisearch）由 Compose 自己拉起，使用命名卷持久化。
- 限流、安全响应头、身份头清洗全部在 Go API 内实现，不依赖代理。

### 2.2 准备环境文件

```sh
cp infra/deploy/.env.example /etc/anby-wiki/.env
sudo chmod 0600 /etc/anby-wiki/.env
```

该文件同时包含普通配置和机密，必须位于仓库之外且仅允许部署用户读取。
Compose 会把机密注入容器环境，因此具有 Docker 管理权限的人可通过
`docker inspect` 查看；不要把环境文件、Compose 展开结果或容器环境写入日志和工单。

必须修改的项：

| 变量 | 说明 |
| --- | --- |
| `RELEASE_ID` | 本地镜像版本标签，只允许字母、数字、点、下划线和连字符 |
| `POSTGRES_DB` `POSTGRES_USER` | PostgreSQL 数据库名和用户 |
| `POSTGRES_PASSWORD` | PostgreSQL 密码；只允许字母、数字、点、下划线和连字符，建议使用 `openssl rand -hex 32` 生成 |
| `S3_ACCESS_KEY` `S3_SECRET_KEY` | 同时作为 MinIO root 凭据与应用 S3 凭据 |
| `MEILI_MASTER_KEY` | 同时作为 Meilisearch Master Key 与应用内部 API Key，必须替换模板占位值 |
| `SEARCH_BACKEND` | 生产保持 `meilisearch`；PostgreSQL 只作为开发 fallback |
| `AUTH_REGISTRATION_ENABLED` | 是否允许公开注册；首个管理员建立后建议设为 `false` |
| `AI_CONFIG_MASTER_KEY` | 32 字节随机密钥的 base64，用于 AES-256-GCM 加密管理员保存的 Provider 密钥；建议 `openssl rand -base64 32` |
| `AI_KERNEL_INTERNAL_TOKEN` | API/Worker 与私网 Semantic Kernel Sidecar 的共享随机令牌 |
| `S3_BUCKET` | 对象存储桶名 |
| `SESSION_COOKIE_SECURE` | 外层提供 HTTPS 时设 `true`，否则 `false` |
| `COLLABORATION_ORIGIN_PATTERNS` | Next.js rewrite 或反向代理改写上游 `Host` 时，填写允许建立协作 WebSocket 的公开 Origin；多个值用逗号分隔，生产建议包含 `https://` 以固定协议 |
| `WEB_BIND` `WEB_PORT` | 对外端口；默认 `127.0.0.1:3000` 只绑本机 |
| `TRUSTED_PROXY_IPS` | 仅在 API 直连对端可信且其传来的 `X-Forwarded-For` 已被清洗时填写；默认留空最安全 |

环境文件必须保持 shell 兼容的 `KEY=VALUE` 格式。生产 `DATABASE_URL` 由 Compose
根据 `POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD` 自动生成，无需重复填写。
`VCS_REF` 和 `BUILD_DATE` 由部署脚本从当前 Git 提交和 UTC 时间自动生成。

### 2.3 在部署机本地构建

Anby Wiki 是商业软件，业务镜像只在当前部署机从当前源码构建，不登录业务
registry、不执行 push，也不会从 registry pull 业务镜像：

```sh
export DEPLOY_ENV_FILE=/etc/anby-wiki/.env
sh scripts/deploy.sh build
```

生成五个带版本的本地镜像：

- `anby-wiki-api:$RELEASE_ID`
- `anby-wiki-worker:$RELEASE_ID`
- `anby-wiki-ai-kernel:$RELEASE_ID`
- `anby-wiki-web:$RELEASE_ID`
- `anby-wiki-migrate:$RELEASE_ID`

`deploy` 会自动再次执行本地增量构建，因此单独运行 `build` 只用于提前确认构建过程。
PostgreSQL、Redis、MinIO、Meilisearch、Alpine 等第三方基础镜像仍会在本机缺失时从其上游拉取。
部署目录必须保留完整且受保护的商业源码与 Docker 构建上下文。

### 2.4 部署

```sh
export DEPLOY_ENV_FILE=/etc/anby-wiki/.env
export DEPLOY_CONFIRM="DEPLOY:$(sed -n 's/^RELEASE_ID=//p' "$DEPLOY_ENV_FILE")"

sh scripts/deploy.sh config     # 校验清单可解析
sh scripts/deploy.sh deploy     # 本地构建并正式发布
```

`deploy` 的执行顺序：

1. 校验 `ENV=production`、`RELEASE_ID`、机密变量和环境文件权限；
2. 从当前源码本地构建五个带 `RELEASE_ID` 标签的业务镜像；
3. 运行 `storage-init` 修正命名卷根目录属主；
4. 启动数据层 postgres / redis / minio 并等待健康；
5. 运行 `minio-init` 创建 bucket 并关闭匿名访问；
6. 执行迁移，再校验迁移版本落在镜像兼容窗口内；
7. 运行 `doctor` 自检；
8. 按 `ai-kernel` → `api` → `worker` → `web` 顺序滚动替换。

任一步失败即中止，不会继续替换应用容器。

#### 2.4.1 外层 HTTPS 代理

外层代理必须指向环境文件中实际的 `WEB_BIND:WEB_PORT`，不能假定端口始终为
3000。宿主机 Nginx 的最小站点配置示例：

```nginx
map $http_upgrade $anbywiki_connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen 443 ssl;
    server_name anbywiki.example.com;

    location / {
        proxy_pass http://127.0.0.1:4444;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $anbywiki_connection_upgrade;
    }
}
```

对应部署环境应设置：

```sh
WEB_BIND=127.0.0.1
WEB_PORT=4444
SESSION_COOKIE_SECURE=true
COLLABORATION_ORIGIN_PATTERNS=https://anbywiki.example.com
```

不要只在一次 `docker compose` 命令中临时覆盖 `WEB_BIND/WEB_PORT`。后续 release
重建 Web 容器时仍会读取部署环境文件；缺失时会回退到 Compose 默认端口，可能与宿主机
其他站点冲突并使滚动部署停在 Web 切换阶段。

修改代理后必须先运行 `nginx -t`，再平滑 reload。部署完成后同时验证：

```sh
curl -fsS https://anbywiki.example.com/healthz
curl -fsS https://anbywiki.example.com/readyz
curl -fsS https://anbywiki.example.com/ | grep '<title>Anby Wiki</title>'
```

容器 healthy 只能证明内部服务正常，不能证明域名没有误指向宿主机上的其他端口或
容器。协作发布还应通过 `COLLABORATION_E2E_BASE_URL=https://...` 的 WSS E2E。

### 2.5 其他命令

```sh
sh scripts/deploy.sh build      # 只在本机构建五个业务镜像
sh scripts/deploy.sh migrate    # 只跑迁移与闸门
sh scripts/deploy.sh doctor     # 只跑自检
sh scripts/deploy.sh rollback   # 切回 RELEASE_ID 对应的已有本地镜像
```

回滚前把环境文件中的 `RELEASE_ID` 改为旧版本，并确认部署机仍保留对应的五个
本地镜像。回滚不会重新构建、不会 pull，也**从不**执行 down 迁移；旧镜像必须
显式兼容线上数据库版本。需要缩表时，先发布一个兼容新旧两版的中间版本。

### 2.6 隔离全 API E2E

全 API 测试会创建、审核、应用、回滚和归档数据，并覆盖站点 AI 配置，只能指向独立的
PostgreSQL database、MinIO bucket、Meilisearch index 和 Redis DB，禁止对生产数据运行。
测试管理员必须是空隔离库迁移后的首个注册账号。

```sh
cd backend
API_E2E_BASE_URL=http://127.0.0.1:14545 \
API_E2E_PASSWORD='<isolated-test-password>' \
API_E2E_RUN_ID='<isolated-run-id>' \
go test ./cmd/api -run 'TestAPI.*E2E$' -count=1 -v

COLLABORATION_E2E_BASE_URL=http://127.0.0.1:14545 \
COLLABORATION_E2E_PASSWORD='<isolated-test-password>' \
go test ./cmd/api -run TestCollaborationE2E -count=1 -v
```

`TestAPIContractE2E` 从 OpenAPI 动态读取全部 operation；除故意不可达模型配置产生的
契约内 502/504 外，任何 5xx 都失败。成功工作流另行验证权威写入、异步投影、治理状态机、
Import 恢复点和双用户协作语义。

### 2.7 备份

数据在命名卷 `pgdata` 与 `miniodata` 中，随 `docker compose down` 保留，
但 `down -v` 会删除。备份脚本见 `scripts/backup/postgres-backup.sh`、
`scripts/backup/object-storage-backup.sh`。

---

## 3. 当前部署限制

以下取舍是当前阶段的显式决定，**公网暴露前必须处理**。
遗留项同时记录在 `Docs/OutstandingIssues.md`。

1. **账号恢复与二次验证尚未提供。**
   本地账号已使用独立用户名、邮箱和 Argon2id 密码哈希，但当前没有邮箱验证、
   忘记密码、MFA 或登录设备管理。公网商业部署前应接入邮件服务并补齐这些能力。

2. **默认无 TLS。**
   Compose 不终结 HTTPS。明文暴露时会话 cookie 和登录凭据会在网络上可见，
   请务必在外层提供 HTTPS，并同步设置 `SESSION_COOKIE_SECURE=true`。

3. **搜索容量与语义质量仍需目标环境验收。**
   生产已接入 Meilisearch，PostgreSQL 只保留开发 fallback 与可重建 staging。
   正式发布前仍需按 ADR-0012 的 10 万页面口径复测吞吐、尾延迟、重建时间、
   模型下载和中文语义召回。

4. **限流为应用层 + Redis 固定窗口。**
   Redis 不可达时**放行**并记日志（可用性优先于严格限流），
   因此限流不能作为唯一的滥用防线。默认不信任 `X-Forwarded-For`，
   在当前 Web → API 拓扑下会按 Web 容器这个直连对端计数；只有上游已可靠清洗
   来源头、且 API 对端被列入 `TRUSTED_PROXY_IPS` 后，才会按最终客户端 IP 分桶。
