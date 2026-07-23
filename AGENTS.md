# AGENTS.md — Anby Wiki 仓库级 Agent 开发约定

> 适用于全仓库。`apps/web/AGENTS.md`、`backend/` 下更深层的约定优先于本文件。
> 设计依据：[Docs/WikiDesignOnePage.md](Docs/WikiDesignOnePage.md)、[Docs/WikiImplementationPlan.md](Docs/WikiImplementationPlan.md)。
> 当前实现与发布阻塞分别见 [Docs/CurrentImplementationStatus.md](Docs/CurrentImplementationStatus.md)、
> [Docs/OutstandingIssues.md](Docs/OutstandingIssues.md)。

## 1. 项目速览

人工与 AI 共同维护的现代百科平台：模块化单体 Go API + 独立 Worker + Next.js Web。
权威数据（Page/Revision/Entity/Claim/Source/Citation/Proposal）只能经领域服务写入；
投影（链接、渲染、搜索）由 Outbox 驱动的 Worker 生成，可丢弃可重建。

## 2. 目录与模块边界

| 路径 | 内容 | 关键约束 |
|---|---|---|
| `apps/web/` | Next.js + TS 前端 | 见 `apps/web/AGENTS.md` |
| `backend/cmd/api` | HTTP API 入口 | 只装配，无业务逻辑 |
| `backend/cmd/worker` | 异步 Worker 入口 | Outbox/Projection/导入 |
| `backend/cmd/migrate` | 迁移 CLI | 显式执行，启动不自动迁移 |
| `backend/internal/<domain>` | 领域模块 | service 是唯一权威写入入口 |
| `backend/internal/platform` | 配置/日志/DB/ID/httpx（错误模型写出） | 不依赖任何领域模块 |
| `backend/migrations` | 预发布初始化 Schema | 只维护 `000001_initial_schema.up/down.sql` |
| `contracts/openapi` | OpenAPI 3.1 权威契约 | 改动流程见 §4 |
| `contracts/generated/typescript` | 生成的 TS 客户端 | **禁止手改** |
| `infra/` | Nginx、本地依赖、部署 | 本地环境 `make infra-up` |
| `Docs/adr` | 架构决策记录 | 编号只增不复用 |
| `Docs/CurrentImplementationStatus.md` | 当前阶段能力汇总 | 实现范围变化时同步更新 |
| `Docs/OutstandingIssues.md` | 发布阻塞与遗留问题 | 关闭条件满足后更新 |

## 3. 常用命令

```bash
make bootstrap      # 安装依赖
make infra-up       # 本地 PostgreSQL/Redis/MinIO/Nginx（需 Docker）
make migrate-up     # 执行迁移
make dev            # API + Worker + Web 并行开发
make lint test      # 质量门禁
make gen-client     # 重新生成 TS 客户端（需 Java，Makefile 已处理 Homebrew keg 路径）
make ci             # 本地等价 CI
```

端到端冒烟（Web → Go API）：启动 API 后 `SMOKE_API_URL=http://localhost:8080 npm run test`（apps/web 目录）。

PostgreSQL 集成测试：用 `make pg-start` 启动免 Docker 本地实例（Homebrew postgresql@17，端口 55432，数据在 /tmp），然后 `TEST_DATABASE_URL=postgres://wiki@127.0.0.1:55432/wiki?sslmode=disable make test-go-integration`（或手动 `go test ./... -count=1 -p 1`）。未设置 `TEST_DATABASE_URL` 时集成测试必须 skip，单元测试不得依赖数据库。**集成用例每例 Reset 全库，多包并行（或多 Agent 共用同一库）会互相 TRUNCATE——必须 `-p 1` 串行；并行开发时请为每个 Agent 建独立库**（`createdb` 后 `cmd/migrate up`）。

## 4. 不可违反的规则

1. **权威写入只经领域服务**：API handler、Worker、Agent 不得直接拼 SQL 改权威状态。
2. **Revision/ContentSnapshot 发布后不可变**；Projection 可丢弃、可重建。
3. **前端 API 调用必须经生成客户端**（`apps/web/lib/api.ts` 工厂函数）；SWR 是服务端数据唯一客户端缓存，Zustand 只存本地交互状态。
4. **契约变更流程**：改 `contracts/openapi` → `make gen-client` → 提交生成物 diff → CI 漂移检查。不得反向手改生成物。
5. **日志禁记**：来源全文、Prompt、密钥、Token、个人敏感信息。
6. Toast=Sonner、图标=Lucide、字体=Geist、命令面板=cmdk、校验=Zod，不引入第二套。

## 5. 高冲突资源所有权（实施方案 §8.2）

以下资源同一时间只分配给一个 Task/Agent，开工前确认无并行持有者：

- 数据库主 Schema（预发布阶段直接修改唯一 `000001_initial_schema.up/down.sql`）；
- Typed Block AST 根 Schema 与 Schema version（`contracts/schemas`）；
- ProposalOperation Union 与事件 Envelope；
- 发布事务、Proposal Apply 事务；
- 仓库根配置、CI、`backend/go.mod`、`apps/web/package-lock.json`。

其他 Agent 基于已合入契约开发 Adapter/UI/Builder/Fixture，不顺手修改公共契约。

## 6. 分支与提交

- 分支应聚焦单一变更；命名使用简洁的 `<area>-<slug>`。
- 提交信息使用 `<area>: <what>`，正文写验收命令与风险。
- 每个提交必须能独立通过 `make ci` 中与其相关的检查。
- 未完成里程碑冻结的 Schema/API/事件不得被下游 Task 当作稳定接口。

## 7. Definition of Done（实施方案 §4.3 摘要）

- 类型检查、Lint、单元测试、相关集成测试通过；
- 新 API/事件/Operation 有版本化 Schema 与契约测试；
- Schema 变更同步维护初始化 up/down，并验证空库 up、完整 down、再次 up；
- 权威写入路径有事务测试，异步路径有幂等与重试测试；
- 实现范围变化时更新 `Docs/CurrentImplementationStatus.md`；
- 不留无负责人 TODO。

## 8. 交接清单（每个 Task 结束时）

1. 变更文件清单与一句话说明；
2. 执行的验证命令与结果（含失败路径测试）；
3. 当前状态/ADR/运维文档的同步更新；
4. 释放的高冲突资源声明；
5. 遗留问题与建议的后续 Task ID。
