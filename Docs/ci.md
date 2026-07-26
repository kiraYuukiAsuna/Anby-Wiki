# CI 与质量门禁

工作流：`.github/workflows/ci.yml`。触发条件为 push / PR 到 `main`，以及手动
`workflow_dispatch`。同一 ref 的新运行会取消进行中的旧运行。

仓库当前不维护自动化单元、集成或浏览器测试套件。CI 聚焦静态分析、构建、契约、
迁移、部署清单和安全扫描。

## 各 Job

| Job | 内容 | 本地复现 |
| --- | --- | --- |
| `backend` | `go mod verify`、gofmt、`go vet ./...`、`go build ./...` | `make lint-go build-go` |
| `web` | `npm ci`、TypeScript、ESLint、Next.js build | `make lint-web build-web` |
| `security` | gitleaks、Go module 校验、govulncheck、npm production audit | `make security` |
| `contracts` | OpenAPI 3.1 校验及权威 Schema 与 Go 内嵌副本字节级漂移检查 | `make contracts-check`，OpenAPI 校验需 JDK |
| `client-drift` | 重新生成 TypeScript 客户端并检查 Git diff | `make gen-check` |
| `migrations` | 迁移文件规范检查，并在空 PostgreSQL 17 上执行 up/version | `make migration-check`、`make migrate-up` |
| `deploy` | Shell 语法、Dockerfile/Compose 静态检查、OCI target 构建和运行元数据检查 | `make deploy-check` |

## 迁移文件规范

`make migration-check` 检查：

- 命名为 `{seq}_{name}.up.sql` / `{seq}_{name}.down.sql`；
- up/down 成对且名称一致；
- 六位序号从 `000001` 连续无跳号；
- 名称为非空小写蛇形；
- 除 `README.md` 外不存在游离文件。

CI 的 `migrations` Job 另起 PostgreSQL 17，对空库真实执行 `go run ./cmd/migrate up`
与 `version`。本地可通过 `.env` 指向自备 PostgreSQL 后运行 `make migrate-up`。

## 部署配置门禁

`make deploy-check` 检查全部 Shell 语法、Dockerfile target、non-root 用户、
Compose 只读根文件系统、capability drop、`no-new-privileges`、环境模板完整性和迁移
版本同步。Docker daemon 可用时追加 `docker compose config --quiet`。

GitHub Actions 的 `deploy` Job 进一步构建 `api`、`worker`、`web`、`migrate` 四个
OCI target，并检查运行用户与 healthcheck 元数据。本地没有 Docker 时，静态检查仍可
运行，但不能替代 CI 的真实镜像构建。

## 安全门禁

工具版本固定在根 `Makefile` 与 CI 中。`npm audit --omit=dev --audit-level=high`
只以 production high/critical 为阻断条件，不使用 `--force` 自动执行破坏性降级。
gitleaks 只豁免 `.next` 本地生成目录。

## 失败排查

1. `backend`：运行 `make lint-go build-go`。
2. `web`：运行 `make lint-web build-web`。
3. `client-drift`：运行 `make gen-check` 并提交生成物。
4. `contracts`：运行 `make contracts-check`，先改权威 Schema 再同步内嵌副本。
5. `migrations`：先运行 `make migration-check`，再对空库运行 `make migrate-up`。
6. `deploy`：运行 `make deploy-check`；OCI 构建问题不得通过改成 root 或移除
   healthcheck 规避。
7. `security`：按扫描报告升级最小兼容版本，不降低审计级别。

一次性执行本地现有门禁：`make ci`。
