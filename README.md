# Anby Wiki

人工与 AI 共同维护的现代百科平台。结构化 Block AST + Page/Revision + Entity/Claim +
Source/Citation + Proposal/Review + Projection/Search。

## 文档

- [整体设计方案](Docs/WikiDesignOnePage.md)
- [实施方案](Docs/WikiImplementationPlan.md)
- [当前实现状态](Docs/CurrentImplementationStatus.md)
- [待解决问题](Docs/OutstandingIssues.md)
- [ADR 索引](Docs/adr/README.md)

## 快速开始

前置依赖：Go 1.24+、Node 20+、Docker（本地基础设施）。

```bash
make bootstrap      # 安装前后端依赖
make infra-up       # 启动 PostgreSQL / Redis / MinIO / Nginx
make migrate-up     # 执行数据库迁移
make dev            # 同时启动 API / Worker / Web（开发模式）
```

## 目录

```text
apps/web/        Next.js 前端
backend/         Go API + Worker（模块化单体）
contracts/       OpenAPI 3.1 契约、JSON Schema、生成客户端
infra/           Nginx、本地依赖、部署模板
Docs/            设计、当前状态、ADR、运维与安全文档
```

## 质量门禁

```bash
make lint         # Go + 前端静态检查
make test         # Go + 前端测试
make gen-client   # 从 OpenAPI 重新生成 TS 客户端（生成物禁止手改）
make ci           # 本地跑完整 CI 等价检查
```
