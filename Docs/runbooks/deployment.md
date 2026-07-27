# Production Deployment and Migration Runbook

本手册是 [Deploy.md](../../Deploy.md) 的生产运维补充。权威清单为
`infra/deploy/compose.production.yml`，发布入口为 `scripts/deploy.sh`。

## 当前拓扑

Production Compose 部署 PostgreSQL、Redis、MinIO、API、Worker 与 Web。
Web 是唯一发布宿主机端口的服务，并经 Next.js rewrite 将 `/api/*` 转发到
Docker 内网 API。清单不包含 Nginx、Meilisearch 或外部身份提供方。

Compose 本身不终结 TLS。默认 `WEB_BIND=127.0.0.1`，适合由宿主机代理、云负载
均衡或隧道接入；如果直接改成公网监听，必须先评估明文登录凭据与 session cookie
风险。完整限制见 `Deploy.md` 与 `Docs/OutstandingIssues.md`。

## 本地构建策略

商业业务镜像不发布到 registry。部署机使用当前受保护源码与 Dockerfile 本地构建
`api`、`worker`、`web`、`migrate` 四个 target，并按 `RELEASE_ID` 标记为
`anby-wiki-<target>:<release>`。Compose 对这些服务设置 `pull_policy: never`。

```sh
export DEPLOY_ENV_FILE=/etc/anby-wiki/.env
sh scripts/deploy.sh build
```

基础数据服务镜像仍来自各自上游；“不发布”仅指 Anby Wiki 自有业务镜像。

## 环境与机密

复制 `infra/deploy/.env.example` 到仓库外的受保护路径并设置 `chmod 0600`。
该文件同时保存普通配置与 `POSTGRES_PASSWORD`、
`S3_ACCESS_KEY`、`S3_SECRET_KEY` 等机密。

Compose 使用 `POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD` 自动生成内部
`DATABASE_URL`，无需重复填写。PostgreSQL 三个值只允许字母、数字、点、下划线
和连字符；密码建议用
`openssl rand -hex 32` 生成。S3 两个值同时作为 MinIO root 凭据与应用凭据。
首次部署时保持 `AUTH_REGISTRATION_ENABLED=true`，从 `/register` 创建首个管理员；
完成所需账号初始化后建议改为 `false` 并重新部署。
启用 `AI_IMPORT_ENABLED` 时，`AI_API_KEY` 同样写入该文件，但只注入 Worker。

Compose 会把这些值注入容器环境，Docker 管理员可通过 `docker inspect` 查看。
不要提交环境文件，也不要把 Compose 展开结果、容器环境、CI 日志、指标或 trace
写入工单。

## 运行时安全

应用与数据服务使用只读根文件系统、非 root 用户、capability drop 与
`no-new-privileges`；持久数据只写命名卷。Go API 接管原边界层职责：

- 普通请求体 2 MiB，上传 envelope 11 MiB，文件内容仍限制 10 MiB；
- auth、upload、general 三类 Redis 固定窗口限流；
- production 清除可伪造的身份头；
- API 添加 CSP、nosniff、frame 与 referrer 安全头。

ADR-0020 已移除 Origin/Referer CSRF 门禁和 COOP/CORP。Session cookie 仍为
HttpOnly、SameSite=Lax；该简化只适用于当前受控早期部署，公网发布前必须重新评估
显式 CSRF 防护。

默认不信任 `X-Forwarded-For`，因此 Web → API 拓扑下限流按 Web 容器这个直连
对端计数。只有上游已可靠清洗来源头，且 API 直连对端 IP 已明确配置到
`TRUSTED_PROXY_IPS` 时，才能按最终客户端 IP 分桶。

## 发布前检查

1. `make deploy-check` 通过。
2. 部署机具有完整受保护源码和足够的本地构建空间。
3. PostgreSQL 与对象存储备份完成。
4. `MIGRATION_EXPECTED_VERSION` 等于仓库最新迁移版本，并落在镜像兼容窗口内。
5. 部署环境文件中的机密变量非空，且文件不允许 group/world 读取。
6. 若外层终结 HTTPS，设置 `SESSION_COOKIE_SECURE=true`。

## 部署与迁移闸门

```sh
export DEPLOY_ENV_FILE=/etc/anby-wiki/.env
export DEPLOY_CONFIRM="DEPLOY:$(sed -n 's/^RELEASE_ID=//p' "$DEPLOY_ENV_FILE")"

sh scripts/deploy.sh config
sh scripts/deploy.sh deploy
```

固定顺序：

1. 校验环境、`RELEASE_ID`、迁移窗口、机密变量与环境文件权限；
2. 从当前源码本地构建四个业务镜像；
3. 运行 `storage-init` 修正命名卷根目录属主；
4. 启动并等待 PostgreSQL、Redis、MinIO；
5. 运行 `minio-init` 创建私有 bucket；
6. 执行 `wiki-migrate up` 与版本兼容检查；
7. 运行 `wiki-doctor -format json`；
8. 依次替换 API、Worker、Web，并等待各自 healthcheck。

任何步骤失败都会阻止后续替换。应用启动本身不自动迁移；禁止在生产执行
`migrate down`。

## 回滚

把环境文件中的 `RELEASE_ID` 改为上一个版本，确认该版本的四个本地镜像仍保留，
并保持线上数据库实际版本与兼容窗口正确，然后执行：

```sh
sh scripts/deploy.sh rollback
```

回滚不会构建、pull 或 push，只检查旧版本本地镜像、运行版本检查和 doctor，
再按 API → Worker → Web 替换；不会执行 up/down 迁移。如果旧镜像不兼容当前
Schema，禁止回滚，必须发布 forward fix。

## 故障处置

- 数据层启动失败：保留卷与容器日志，修复用户、tmpfs、环境变量或卷权限后重试。
- `minio-init` 失败：确认 root 凭据与应用 S3 凭据一致，bucket 名合法。
- 迁移失败或 dirty：停止发布，在备份恢复副本上准备幂等前向修复。
- doctor 返回 error/critical：按
  [data-consistency-doctor.md](./data-consistency-doctor.md) 处理后重跑。
- 应用 healthcheck 失败：不要手动跳过；确认配置、依赖与镜像内 health 工具。

Docker 不可用的开发机只能完成静态清单和 Shell 语法检查，不能作为真实镜像/Compose
通过的证据；CI deploy job 或具备 Docker daemon 的发布机仍需执行真实校验。
