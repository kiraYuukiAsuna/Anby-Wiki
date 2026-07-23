# Typed Block AST v1

Wiki 页面正文的权威格式（设计依据：`Docs/WikiDesignOnePage.md` §4.9、§5、§5.1-5.2）。
本目录是 AST 的语言无关公共契约，下游编辑器、Renderer、Patch Engine、Proposal 均以此为准。

- 权威 Schema：`ast.schema.json`（JSON Schema draft 2020-12，`$id: https://anby.wiki/schemas/ast/v1/ast.schema.json`）
- 校验样例：`fixtures/valid/*.json`（必须全部通过）、`fixtures/invalid/*.json`（必须全部拒绝）
- 序列化样例：`fixtures/canonical/{name}.json` + `{name}.canonical.json` + `{name}.sha256`（Go `CanonicalizeJSON` 与 Zod 侧 `canonicalJson` 必须逐字节一致，哈希为前者的 SHA-256；规则见 `backend/internal/ast/serialize.go` 头注释）
- 语言绑定：`backend/internal/ast`（Go，`santhosh-tekuri/jsonschema/v6` 校验）、`apps/web/lib/ast/schema.ts`（Zod v4）

## 结构概要

- 根节点：`{ type: "document", schema_version: 1, children: Block[] }`。
- Block 是按 `type` 判别的联合：`heading`（level 1-6）、`paragraph`、`bullet_list` / `ordered_list` / `list_item`、`table` / `table_row` / `table_cell`、`code`（language 可选）、`quote`、`callout`（kind: info/warning/danger）、`divider`。
- 容器规则：
  - `bullet_list` / `ordered_list` 的 `children` 只能是 `list_item`；
  - `table` → `table_row` → `table_cell` 逐层包裹；
  - `list_item` / `table_cell` / `quote` / `callout` 的 `children` 为任意 Block（嵌套 list 仍必须经 `list_item` 包裹）；
  - `heading` / `paragraph` 持有 `content: InlineNode[]`，没有 `children`；
  - `code` 持有 `content: string`；
  - `divider` 无任何内容字段。
- Inline 节点：`text`（可选 `marks`：bold/italic/strikethrough/code）、`inline_code`、`page_reference`、`external_link`、`entity_reference`、`claim_reference`、`citation_reference`。
- `page_reference` 两种互斥形态（`oneOf`）：
  - 已解析：`target_page_id`（uuid）+ `display_text`，可选 `target_heading_block_id`（章节锚点，设计 §9）；
  - 未解析：`resolution_status: "unresolved"` + `target_namespace` + `normalized_title`，可选 `expected_entity_type`。
  - 页面改名不修改正文中的引用（设计 §5.1）；未解析引用由后台 Resolver 在新页面创建后自动解析（设计 §5.2）。
- `external_link` 直接保存 `url` + `display_text`；`external_resource_id` 规范化属于 M3，不在 v1。
- 知识引用（M4-T06）只保存稳定 ID 与展示兜底，不把领域对象快照嵌入正文：
  - `entity_reference`：`entity_id` + `display_text`；
  - `claim_reference`：`claim_id` + `display_text`；
  - `citation_reference`：`citation_id` + 可选 `display_text`（Renderer 用作文献提示）。
- 所有对象均 `additionalProperties: false`，严格校验。

## ID 规则（ADR-0008）

- 每个 Block 必填 `id`，`format: uuid`，使用 **UUIDv7**，标准 36 字符小写连字符形式。
- ID 由编辑器 Adapter / AI Patch Engine 在客户端或服务端应用层生成，不依赖数据库默认值。
- Block ID 稳定：块移动、改名、跨版本复用均不改变 ID；块级 Diff、评论、冲突检测、来源绑定都以此为准。
- Go 侧生成入口：`backend/internal/ast.NewID()`（封装 `platform/id`）；前端使用 `uuid` npm 包 v9+ 的 v7 生成器。

## 版本规则

- `schema_version` 为 const `1`。
- **v1 只允许 additive 演进**，以下变更可在 v1 内进行（同步更新 Go/Zod 绑定与 fixtures）：
  - 新增 Block 类型（在 `block` 的 `oneOf` 追加分支，使用新的 `type` 判别值）；
  - 新增 Inline 节点类型（在 `inlineNode` 的 `oneOf` 追加分支；M4-T06 的 EntityReference / ClaimReference / CitationReference 即按此方式扩展）；
  - 为已有节点新增**可选**字段；
  - 新增 `marks` / `kind` 枚举值（消费方必须容忍未知枚举或同步升级）。
- 以下属于 **breaking change**，必须升级 `schema_version` 到 2 并新建 `contracts/schemas/ast/v2/`：
  - 删除或重命名字段、类型、枚举值；
  - 可选字段改必填、收窄已有字段的取值范围；
  - 改变容器规则或判别字段语义。
- 迁移与回放工具必须能同时读取仍存量的 v1 文档与新版文档。

## 扩展指南（以新增 Inline 节点为例）

1. 在 `ast.schema.json` 的 `$defs` 新增节点定义（`type` const 判别 + `additionalProperties: false`），并加入 `inlineNode` 的 `oneOf`。
2. `backend/internal/ast`：在 Go 类型中补充对应常量/字段；将最新 Schema 同步到 `backend/internal/ast/schema/ast.schema.json`（见下）。
3. `apps/web/lib/ast/schema.ts`：新增等价 Zod schema 并加入 `inlineNodeSchema` union。
4. 在 `fixtures/valid/` 增加覆盖新节点的样例；若新增校验规则，在 `fixtures/invalid/` 增加反例。
5. 跑通两侧测试：`cd backend && go test ./internal/ast/...`；`cd apps/web && npm run typecheck && npm run test`。

## Schema 同源与漂移防护

`go:embed` 不能跨 Go module 边界引用 `../../contracts`，因此采用副本 + 自动化校验：

- `backend/internal/ast/schema/ast.schema.json` 是 `ast.schema.json` 的字节级副本（Go 侧 `go:embed` 用）。
- `scripts/check-ast-schema-sync.sh` 校验两份文件字节一致（供 CI / 手工调用）。
- `backend/internal/ast` 的单测内嵌副本与 `contracts/` 原文件做一致性断言，`go test` 即暴露漂移。

修改 Schema 时始终先改 `contracts/schemas/ast/v1/ast.schema.json`，再同步副本。

## 已知实现差异

- `external_link.url`：JSON Schema 用 `format: uri`（RFC 3986，允许相对引用）；Zod 侧用 `z.url()`（要求绝对 URL）。生产方应始终写绝对 URL，fixtures 与测试均按绝对 URL 覆盖。
