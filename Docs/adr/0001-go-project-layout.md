# ADR-0001：Go 工程结构与模块边界

状态：已接受（M0-T02）
日期：2026-07-21

## 背景

实施方案 §2 固定了 Go + TypeScript 单仓库，但 Go 工程的具体结构、模块划分和依赖规则需要固化。

## 决策

- 仓库根下 `backend/` 为**单一 Go Module**（`github.com/anby/wiki/backend`），不为 cmd/api 与 cmd/worker 建独立 module。
- 目录：
  - `backend/cmd/api`：HTTP API 入口（只装配，不含业务逻辑）。
  - `backend/cmd/worker`：异步 Worker 入口（Outbox、Projection、导入）。
  - `backend/internal/<domain>`：领域模块，首批为 `page`、`knowledge`、`evidence`、`governance`、`projection`，另有 `platform`（配置、日志、DB、HTTP 基础设施）。
  - `backend/internal/<domain>` 内部固定分层：`service`（领域服务，唯一权威写入入口）、`repository`（SQL）、`api`（HTTP handler）、`events`（Outbox 载荷）。
  - `backend/migrations`：显式 SQL 迁移（见 ADR-0002）。
  - `backend/testkit`：Fixture、Factory、集成测试工具。
- 依赖规则（由 `go vet` + 代码评审执行，后续可接 `go-arch-lint`）：
  - `cmd/*` 可以依赖所有 `internal/*`；`internal/*` 之间允许通过**接口**单向依赖，`platform` 不依赖任何领域模块。
  - HTTP handler、Worker 不得直接调用 `repository` 修改权威状态，必须经 `service`。
- 前端位于 `apps/web`（Next.js），契约位于 `contracts/`，基础设施位于 `infra/`，与实施方案 §2 目录边界一致。

## 备选方案

- 多 Go Module 仓库：过早增加版本管理成本，P0 单仓库单 module 足够。
- `pkg/` 公共目录：Go 社区已不推荐，复用代码放 `internal/platform`。

## 影响

- 模块化单体 + 独立 Worker 的部署形态被结构固化；拆服务时按 `internal/<domain>` 边界切分。
- 仓库中已存在的空目录 `Frontend/`、`Server/` 废弃删除，以本结构为准。
