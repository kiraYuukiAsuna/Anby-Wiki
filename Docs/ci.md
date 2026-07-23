# CI 与质量门禁

工作流：`.github/workflows/ci.yml`（GitHub Actions）。触发条件：push / PR 到 `main`，以及手动 `workflow_dispatch`。同一 ref 的新运行会取消进行中的旧运行（concurrency）。所有第三方 action 固定到 major tag。

## 各 Job 说明

| Job | 内容 | 本地复现 |
| --- | --- | --- |
| `backend` | `gofmt -l` 非空即失败、`go vet ./...`、`go test ./...`（单元）、对 postgres 服务容器执行迁移后 `TEST_DATABASE_URL=... go test ./... -count=1`（PostgreSQL 集成）、`go build ./...`（工作目录 `backend/`，Go 版本取自 `backend/go.mod`） | `make lint-go`、`make test-go`、`make build-go`；集成测试见下节 |
| `web` | `npm ci` 后依次 `npm run typecheck` / `lint` / `test` / `build`（工作目录 `apps/web/`，Node 22） | `make lint-web`、`make test-web`、`make build-web`；类型检查为 `cd apps/web && npm run typecheck` |
| `security` | `gitleaks v8.28.0` 密钥扫描、`go mod verify`、`govulncheck v1.1.4`、npm production high audit | `make security`，或分别执行 `make security-go security-web security-secrets` |
| `browser-e2e` | 启动独立 PostgreSQL 17、执行迁移、安装 Chromium，再由 Playwright 自动启动 Go API 与 Next.js，完成页面浏览器全生命周期（MT-M2-BROWSER-LIFECYCLE） | `make pg-start`（已自动迁移）后，`cd apps/web && npx playwright install chromium && npm run test:e2e`；使用系统 Chrome 可设 `PLAYWRIGHT_CHANNEL=chrome` |
| `contracts` | 校验 OpenAPI 3.1，并运行 AST / ProposalOperation 权威 Schema 与 Go 内嵌副本的字节级防漂移检查 | `make contract-schema-check`；`cd apps/web && npm ci && npx @openapitools/openapi-generator-cli validate -i ../../contracts/openapi/openapi.yaml`（本地需 JDK） |
| `client-drift` | JDK 21 + `npm ci` 后重新执行 `npm run gen:client`，再 `git diff --exit-code contracts/generated/typescript`；生成物与仓库不一致即失败 | `make gen-check`（本地需 JDK） |
| `migrations` | 起 `postgres:17` 服务容器（库/用户均为 `wiki`），先 `sh scripts/check-migrations.sh` 校验迁移文件规范，再对空库执行 `go run ./cmd/migrate up` 与 `version` | `sh scripts/check-migrations.sh`；`make infra-up && make migrate-up` |
| `deploy` | 校验 Dockerfile/Compose/shell、迁移目标、失败停止和发布顺序；实构建 `api`/`worker`/`web`/`migrate` 四 target 并检查 non-root/health metadata | `make deploy-config-check`；Docker 可用时按 `Docs/runbooks/deployment.md` 实构建 |

### backend job 的 PostgreSQL 集成测试

M1-T08 起，`backend` job 起 `postgres:17` 服务容器（库/用户均为 `wiki`，与
`migrations` job 同一配置），分两步跑测试：

1. `go test ./...`——单元测试；未设置 `TEST_DATABASE_URL` 时集成用例由
   testkit 自动 skip（保证纯单元层面也绿）。
2. `go run ./cmd/migrate up`（`DATABASE_URL` 指向服务容器，`REDIS_URL`/`S3_*`
   为 config.Load 必填的占位值）后，以
   `TEST_DATABASE_URL='postgres://wiki:wiki@localhost:5432/wiki?sslmode=disable'`
   执行 `go test ./... -count=1`——发布原子性、不可变触发器、里程碑生命周期等
   PG 集成测试真实跑在 CI 上（核心不变量门禁必须在 CI 常绿，不能接受 skip）。

本地复现：`make pg-start`（免 Docker，端口 55432）后在 `backend/` 下
`TEST_DATABASE_URL='postgres://wiki@127.0.0.1:55432/wiki?sslmode=disable' go test ./... -count=1`。

### browser-e2e job 的真实浏览器验收

`apps/web/e2e/page-lifecycle.spec.ts` 不直接请求内部 API，也不构造 AST JSON：它从
Playwright browser context 的 `extraHTTPHeaders` 显式注入仅 test 环境启用的种子 Actor，
经 `/new` 创建引用目标和正文页面，使用 Block Editor
完成段落、标题、列表、表格和内部链接编辑，再验证发布、改名与旧别名、双浏览器上下文
并发冲突、历史 Diff 和回滚。Playwright 串行运行，API 与 Web 由
`playwright.config.ts` 自动启停；CI 使用独立 PostgreSQL 服务容器，避免污染其他 Job。

本机已有 Google Chrome 时可免装 Playwright Chromium：
`cd apps/web && PLAYWRIGHT_CHANNEL=chrome npm run test:e2e`。默认数据库为
`postgres://wiki@127.0.0.1:55432/wiki?sslmode=disable`，可用
`E2E_DATABASE_URL` 覆盖。

`vitest.config.ts` 显式排除 `e2e/**`：Playwright 的 `*.spec.ts` 只能由
`npm run test:e2e` 执行，避免普通 `npm test` 把浏览器套件当作 jsdom 单测加载。

### migrations job 的空迁移阶段

`cmd/migrate` 只把 `ErrNoChange` 视为成功，但经 golang-migrate v4.19.1 源码与本地小程序验证：**迁移目录为空（或仅 README.md）时，`up` 返回的是 source 层的 `fs.ErrNotExist`（"file does not exist"），不是 `ErrNoChange`**。因此 CI 中该步骤按 `migrations/*.up.sql` 是否存在做条件执行：无迁移文件时跳过 `up`/`version`（仅跑命名校验脚本），首个迁移合入后自动转为真实执行。

另外 `cmd/migrate` 经 `config.Load()` 读配置，`REDIS_URL`、`S3_*` 也是必填项，CI 步骤中为它们填了占位值（本步骤只使用 `DATABASE_URL`）。

## 迁移文件规范（scripts/check-migrations.sh）

- 命名 `{seq}_{name}.up.sql` / `{seq}_{name}.down.sql`，up/down 必须成对且 `name` 一致；
- `{seq}` 恰好 6 位数字，全局唯一，从 `000001` 连续无跳号；
- `{name}` 为非空小写蛇形（`[a-z0-9_]`，不允许首尾下划线）；
- 除 `README.md` 外不允许游离文件；
- 空目录（或仅 README.md）通过。

脚本为 POSIX sh，可在仓库任意位置运行：`sh scripts/check-migrations.sh [可选目录]`。

## 部署配置门禁

`make deploy-config-check` 始终执行 POSIX shell 语法、四 target、non-root、
Compose 只读/cap-drop/no-new-privileges、最新迁移版本与示例 gate 版本同步、
显式 seed 保护，以及 migrate/check/doctor -> API -> Worker -> Web -> Nginx
顺序和失败停止测试。Docker daemon 可用时追加 `docker compose config --quiet`。

GitHub Actions 的 `deploy` job 进一步真实构建四个 OCI target，并通过 image
metadata 校验运行用户和 API/Worker/Web healthcheck。开发机没有 Docker 时静态
检查保持可运行，但输出 `BLOCKED real compose/image validation`；这不等价于真实
镜像通过，CI `deploy` job 仍是发布前强制门禁。

## 失败排查

1. 看 job 日志定位失败步骤，用上表"本地复现"列的命令在本地重跑。
2. `client-drift` 失败：本地 `make gen-check`，把重新生成的 `contracts/generated/typescript` 一并提交。
3. `contracts` 失败：先跑 `make contract-schema-check`；ProposalOperation 必须先改 `contracts/schemas/proposal-operation/v1/operation.schema.json`，再同步 `backend/internal/governance/schema/operation.schema.json`。
4. `migrations` 失败：先跑 `sh scripts/check-migrations.sh` 看命名/成对/连续性；再对本地空库 `make migrate-up` 验证 SQL 本身。
5. `deploy` 失败：先跑 `make deploy-config-check`；迁移 gate/顺序失败不可跳过。OCI build 失败按具体 target 修复，禁止改成 root、可写根文件系统或移除 healthcheck 规避。
6. `security` 失败：真实漏洞不得加入白名单。Go 按报告中的 `Fixed in` 升级最小依赖；npm 先确认修复是否兼容当前 Next major；gitleaks 用脱敏 JSON 报告定位后，只对已确认的生成物或公开测试 fixture 添加精确路径豁免。

## 安全门禁

工具版本固定在根 `Makefile` 与 CI 命令中，升级版本必须本地先完整实跑。`npm audit --omit=dev --audit-level=high` 只以 production 依赖的 high/critical 为阻断条件，不使用 `--force` 自动执行破坏性 major/downgrade。

2026-07-23 基线状态：

- `go mod verify` 与 `govulncheck v1.1.4` 通过；已升级修复 `x/text` 与 `go-jose` 的可达漏洞。
- `gitleaks v8.28.0` 通过；`.gitleaks.toml` 仅排除 `.next` 生成目录和包含公开固定幂等 UUID 的单个测试文件。
- npm production audit 仍被 Next `16.2.11` 的 `sharp 0.34.5` high 漏洞阻断；详见 `Docs/security.md`。在兼容修复版出现前 CI 保持红灯，不降低 audit level。

## 如何新增检查

- 新增 Go/前端检查：先在 `Makefile` 或 `apps/web/package.json` 加对应目标，再在 `ci.yml` 的 `backend`/`web` job 中追加同名步骤，保持 CI 与本地命令一一对应。
- 新增独立门禁（如新服务、新契约）：在 `ci.yml` 增加一个 job，复用 `actions/checkout@v4`、`actions/setup-go@v5`（`go-version-file: backend/go.mod`）、`actions/setup-node@v4`（`cache: npm` + `cache-dependency-path`）的既有写法；需要 Java 的步骤照抄 `setup-java@v4`（temurin 21）。第三方 action 一律固定 major tag。
- 一次性全量本地自检：`make ci`（等价 lint + test + build + gen-check）。
