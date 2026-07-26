# Production Deployment and Migration Runbook

本手册是 [Deploy.md](../../Deploy.md) 的生产运维补充。权威清单为
`infra/deploy/compose.production.yml`，发布入口为 `scripts/deploy.sh`。

## 当前拓扑

Production Compose 部署 PostgreSQL、Redis、MinIO、API、Worker 与 Web。
Web 是唯一发布宿主机端口的服务，并经 Next.js rewrite 将 `/api/*` 转发到
Docker 内网 API。清单不包含 Nginx、Meilisearch 或外部身份提供方。

Compose 本身不终结 TLS。默认 `WEB_BIND=127.0.0.1`，适合由宿主机代理、云负载
均衡或隧道接入；如果直接改成公网监听，必须先评估明文引导令牌与 session cookie
风险。完整限制见 `Deploy.md` 与 `Docs/OutstandingIssues.md`。

## 构建与发布镜像

四个 OCI target 使用同一 release metadata 构建：

```sh
VERSION=2026.07.26.1
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

生产环境文件中的四个 `*_IMAGE` 必须是完整 `name@sha256:...` 引用；
`scripts/deploy.sh` 拒绝 `latest` 和其他可变 tag。

## 环境与机密

复制 `infra/deploy/.env.example` 到仓库外的受保护路径并设置 `chmod 0600`。
该文件同时保存普通配置与 `DATABASE_URL`、`POSTGRES_PASSWORD`、
`S3_ACCESS_KEY`、`S3_SECRET_KEY`、`AUTH_DEV_LOGIN_TOKEN` 等机密。

`DATABASE_URL` 必须使用 Compose 服务名 `postgres`，其密码与
`POSTGRES_PASSWORD` 一致。S3 两个值同时作为 MinIO root 凭据与应用凭据。
引导登录令牌必须为强随机值；该登录方式不验证真实身份，只允许封闭早期环境使用。
启用 `AI_IMPORT_ENABLED` 时，`AI_API_KEY` 同样写入该文件，但只注入 Worker。

Compose 会把这些值注入容器环境，Docker 管理员可通过 `docker inspect` 查看。
不要提交环境文件，也不要把 Compose 展开结果、容器环境、CI 日志、指标或 trace
写入工单。密码包含特殊字符时使用 shell 兼容的单引号，并对 URL 密码做 URL 编码。

## 运行时安全

应用与数据服务使用只读根文件系统、非 root 用户、capability drop 与
`no-new-privileges`；持久数据只写命名卷。Go API 接管原边界层职责：

- 普通请求体 2 MiB，上传 envelope 11 MiB，文件内容仍限制 10 MiB；
- auth、upload、general 三类 Redis 固定窗口限流；
- production 清除可伪造的身份头；
- API 添加 CSP、nosniff、frame、referrer 与跨源安全头；
- 携 session cookie 的写请求执行精确 Origin/Referer 校验。

默认不信任 `X-Forwarded-For`，因此 Web → API 拓扑下限流按 Web 容器这个直连
对端计数。只有上游已可靠清洗来源头，且 API 直连对端 IP 已明确配置到
`TRUSTED_PROXY_IPS` 时，才能按最终客户端 IP 分桶。

## 发布前检查

1. `make deploy-check` 通过。
2. 四个应用镜像均已推送并解析到 digest。
3. PostgreSQL 与对象存储备份完成。
4. `MIGRATION_EXPECTED_VERSION` 等于仓库最新迁移版本，并落在镜像兼容窗口内。
5. 部署环境文件中的机密变量非空，且文件不允许 group/world 读取。
6. `TRUSTED_ORIGINS` 与实际用户 origin 完全一致。
7. 若外层终结 HTTPS，设置 `SESSION_COOKIE_SECURE=true`。

## 部署与迁移闸门

```sh
export DEPLOY_ENV_FILE=/etc/anby-wiki/.env
export DEPLOY_CONFIRM="DEPLOY:$(sed -n 's/^RELEASE_ID=//p' "$DEPLOY_ENV_FILE")"

sh scripts/deploy.sh config
sh scripts/deploy.sh deploy
```

固定顺序：

1. 校验环境、digest、迁移窗口、机密变量与环境文件权限；
2. 运行 `storage-init` 修正命名卷根目录属主；
3. 启动并等待 PostgreSQL、Redis、MinIO；
4. 运行 `minio-init` 创建私有 bucket；
5. 执行 `wiki-migrate up` 与版本兼容检查；
6. 运行 `wiki-doctor -format json`；
7. 依次替换 API、Worker、Web，并等待各自 healthcheck。

任何步骤失败都会阻止后续替换。应用启动本身不自动迁移；禁止在生产执行
`migrate down`。

## 回滚

把环境文件中的应用 digest 和 `RELEASE_ID` 改为上一个版本，但保持线上数据库
实际版本与兼容窗口正确，然后执行：

```sh
sh scripts/deploy.sh rollback
```

回滚只运行版本检查与 doctor，再按 API → Worker → Web 替换；不会执行 up/down
迁移。如果旧镜像不兼容当前 Schema，禁止回滚，必须发布 forward fix。

## 故障处置

- 数据层启动失败：保留卷与容器日志，修复用户、tmpfs、环境变量或卷权限后重试。
- `minio-init` 失败：确认 root 凭据与应用 S3 凭据一致，bucket 名合法。
- 迁移失败或 dirty：停止发布，在备份恢复副本上准备幂等前向修复。
- doctor 返回 error/critical：按
  [data-consistency-doctor.md](./data-consistency-doctor.md) 处理后重跑。
- 应用 healthcheck 失败：不要手动跳过；确认配置、依赖与镜像内 health 工具。

Docker 不可用的开发机只能完成静态清单和脚本测试，不能作为真实镜像/Compose
通过的证据；CI deploy job 或具备 Docker daemon 的发布机仍需执行真实校验。
