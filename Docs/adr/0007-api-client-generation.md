# ADR-0007：API 客户端生成（OpenAPI Generator）

状态：已接受（M0-T02）
日期：2026-07-21

## 背景

实施方案要求 OpenAPI 3.1 是唯一权威契约，OpenAPI Generator 或 Kiota 二选一、固定版本、只维护一条生成链路、生成物禁止手改。

## 决策

- 选定：**OpenAPI Generator v7**（generator：`typescript-fetch`），以 `@openapitools/openapi-generator-cli` 的固定版本通过 npm script 调用，版本锁定在 `apps/web/package.json`。
- 生成物输出至 `contracts/generated/typescript/`，目录由 `.gitignore` 排除、`make gen-client` 重新生成；CI 做**漂移检查**（重新生成后 `git diff --exit-code` 不允许有差异——生成物不入库时改为校验生成可成功且类型检查通过）。
  - 补充决定：生成物**提交入库**，便于审查 diff 与离线开发；CI 漂移检查强制其与契约一致。
- 前端所有 API 调用经生成客户端 + 薄封装 `apps/web/lib/api.ts`（注入 baseURL、请求 ID、错误规范化），页面/Store 不手写 URL 与 DTO。
- Go 侧**不生成 server stub**：handler 手写，由 OpenAPI Contract Test（M7 前逐步覆盖）保证一致性；减少双端生成物维护面。

## 备选方案

- Kiota：fluent 体验好，但 TS 生态与社区例少于 OpenAPI Generator。
- openapi-typescript（类型即契约）：更轻，但不在实施方案允许的二选一内，且只生成类型不生成运行时客户端。

## 影响

- M0-T06 验证「Next.js 经生成客户端调用 Go API 健康检查」全链路。
- 契约变更流程：改 `contracts/openapi` → 重新生成 → 提交生成物 diff → CI 校验。
