# Anby Wiki

人工与 AI 共同维护的现代百科平台。结构化 Block AST + Page/Revision + Entity/Claim +
Source/Citation + Proposal/Review + Projection/Search。

## 文档

- [整体设计方案](Docs/WikiDesignOnePage.md)
- [实施方案](Docs/WikiImplementationPlan.md)
- [当前实现状态](Docs/CurrentImplementationStatus.md)
- [待解决问题](Docs/OutstandingIssues.md)
- [Agent JSON CLI](Docs/AgentCLI.md)
- [开发与部署指南](Deploy.md)
- [ADR 索引](Docs/adr/README.md)

## 快速开始

前置依赖：Go、Node.js，以及可连接的 PostgreSQL / Redis / S3 兼容对象存储。
开发环境不使用 Docker——这三个组件由你自行提供，连接信息写进 `.env`。

```bash
cp .env.example .env   # 填入外部依赖连接串
make bootstrap                 # 安装前后端依赖
make dev                       # 迁移并启动 API / Worker / Web
```

详细说明（含生产 Docker 部署）见 [Deploy.md](Deploy.md)。

## 目录

```text
apps/web/        Next.js 前端
backend/         Go API + Worker（模块化单体）
backend/cmd/anby-wiki/  Agent JSON CLI
contracts/       OpenAPI 3.1 契约、JSON Schema、生成客户端
infra/deploy/    生产 Compose 清单与部署模板
Docs/            设计、当前状态、ADR、运维与安全文档
```

## 质量门禁

```bash
make help         # 查看按职责分组的全部公共命令
make check        # 静态检查、单测、构建、契约与部署脚本检查
make gen-client   # 从 OpenAPI 重新生成 TS 客户端（生成物禁止手改）
make ci           # check + 生成物漂移 + 安全扫描
```
