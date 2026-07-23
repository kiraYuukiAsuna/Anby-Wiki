# 现代 Wiki 整体设计方案

## 1. 系统定位

本系统是一个面向人工与 AI 共同维护的现代百科平台，核心能力包括：

* 人工创建、编辑、审核和维护百科页面；
* 多人实时协作编辑同一页面；
* LLM 从网页、PDF、图片、API、数据库等来源导入信息；
* 自动识别实体、属性、关系和来源证据；
* 自动创建新页面或更新已有页面；
* 自动修改已有段落、链接、引用、信息框和分类；
* 页面、实体和事实均可追踪、审核、回滚；
* 支持中型规模的十万至百万级页面系统；
* 保留未来扩展到更大规模的演进空间。

系统不将 Wikitext 作为内部权威格式，而采用：

```text
Typed Block AST
+ Page / Revision
+ Entity / Claim
+ Source / Citation
+ Proposal / Review
+ Projection / Search
```

MediaWiki 中，`page` 负责稳定页面身份、标题和当前 Revision 指针；每次编辑产生独立 `revision`，内容通过 `slots` 和 `content` 与版本关联。
本设计继承这套版本与治理思想，但将正文升级为结构化 Block AST，并增加知识图谱、证据、多人协作和 AI 自动更新能力。

---

# 2. 总体架构

系统在逻辑上分为六个平面。

```text
┌──────────────────────────────────────┐
│ 文档平面 Document Plane              │
│ Page / Revision / ContentSnapshot    │
│ Typed Block AST                      │
└───────────────────┬──────────────────┘
                    │ describes / mentions
                    ▼
┌──────────────────────────────────────┐
│ 知识平面 Knowledge Plane             │
│ Entity / Property / Claim            │
│ Collection / Dataset                 │
└───────────────────┬──────────────────┘
                    │ supported by
                    ▼
┌──────────────────────────────────────┐
│ 证据平面 Evidence Plane              │
│ Source / SourceVersion / Citation    │
│ ExternalResource / Asset             │
└───────────────────┬──────────────────┘
                    │ changed through
                    ▼
┌──────────────────────────────────────┐
│ 变更平面 Change Plane                │
│ Proposal / Review / Conflict         │
│ ChangeBatch / ImportJob              │
└───────────────────┬──────────────────┘
                    │ publishes
                    ▼
┌──────────────────────────────────────┐
│ 投影平面 Projection Plane            │
│ Links / Mentions / Outline / Render  │
│ Search / Activity / Dependencies     │
└───────────────────┬──────────────────┘
                    │ accessed under
                    ▼
┌──────────────────────────────────────┐
│ 治理平面 Governance Plane            │
│ Actor / Permission / Protection      │
│ Audit / Tag / Patrol / Rollback      │
└──────────────────────────────────────┘
```

---

# 3. 核心设计原则

## 3.1 权威数据来源唯一

系统中的权威数据为：

```text
Page
Revision
ContentSnapshot
Entity
Claim
Source
Citation
Proposal
AuditEvent
```

以下数据属于可重建投影：

```text
PageLinkProjection
EntityMentionProjection
ExternalLinkUsage
CitationUsage
ClaimUsage
ComponentDependency
DocumentOutlineProjection
RenderedPage
SearchDocument
ActivityProjection
```

投影只能由权威数据生成，不能作为反向修改正文或知识的入口。

例如：

```text
Block AST 新增页面引用
        ↓
发布 Revision
        ↓
Projection Builder 解析 AST
        ↓
生成 PageLinkProjection
```

不能通过直接插入 `page_link_projection` 来改变页面内容。

## 3.2 稳定 ID 与显示名称分离

以下对象必须使用稳定 ID：

* Page；
* Entity；
* Block；
* Heading；
* Claim；
* Source；
* Citation；
* ExternalResource；
* Asset；
* Component。

标题、别名、显示文字和 URL Slug 都是可修改属性。

```text
Page 改名       → Page ID 不变
Entity 改名     → Entity ID 不变
章节改名        → Heading Block ID 不变
外部 URL 跳转   → ExternalResource ID 不变
```

## 3.3 正式版本不可变

Revision 和 ContentSnapshot 一旦发布后不再修改。

任何正文变更都产生：

```text
新 ContentSnapshot
+ 新 Revision
```

回滚不是修改旧 Revision，而是基于旧内容创建一个新的 Revision。

## 3.4 AI 只提交变更提议

LLM 不直接：

* 执行数据库写入；
* 覆盖当前 Revision；
* 修改链接投影；
* 修改搜索索引；
* 静默覆盖人工验证事实。

LLM 输出结构化 Proposal，由领域服务进行：

```text
Schema 校验
权限检查
实体消歧
版本检查
冲突检测
风险评估
审核或自动合并
```

## 3.5 文档、知识和展示分离

```text
正文叙述      → ContentSnapshot / Block AST
结构化事实    → Entity / Claim
来源证据      → Source / Citation
信息框展示    → Component
查询和列表    → Collection / Dataset View
```

正文中不重复存储信息框中的所有结构化字段。

---

# 4. 核心领域对象

## 4.1 WikiSite

表示一个独立 Wiki 站点。

```text
WikiSite
├── Namespace
├── Page
├── Entity
├── Collection
├── Actor
└── SiteSettings
```

首版可以只支持一个 WikiSite，但数据库保留 `wiki_id`，避免未来多站点改造。

## 4.2 Namespace

命名空间用于隔离不同类型页面：

```text
Main
Talk
User
Project
Component
Collection
File
```

页面路由唯一性由以下组合确定：

```text
wiki_id
+ namespace_id
+ normalized_title
```

## 4.3 Page

Page 是稳定的页面身份，而不是正文内容。

Page 保存：

* 所属 Wiki；
* 命名空间；
* 当前标题；
* 显示标题；
* 页面语言；
* 页面状态；
* 内容模型；
* 当前 Revision；
* 可选主实体。

```text
Page
├── PageAlias
├── PageRedirect
├── Revision History
├── PageEntityBinding
├── PageProtection
└── Current Revision
```

页面改名时：

```text
更新 Page.normalized_title
更新 Page.display_title
写入旧标题到 PageAlias
Page ID 保持不变
```

## 4.4 PageAlias

保存页面曾用标题、人工别名或导入别名。

它用于：

* 旧 URL 继续访问；
* 页面改名后的跳转；
* LLM 实体和页面匹配；
* 标题搜索；
* 解决同一页面的多种名称。

## 4.5 PageRedirect

表示一个重定向页面。

目标可以是：

* 已存在的 Page；
* Page 的某个章节；
* 尚未解析的命名空间和标题；
* 外部 Wiki 目标。

重定向 Page 自身仍是一个正常 Page，可以拥有 Revision 和审计历史。

## 4.6 Revision

Revision 表示一次正式发布的页面版本。

保存：

* 所属 Page；
* 父 Revision；
* ContentSnapshot；
* 编辑者 Actor；
* 修改摘要；
* ChangeBatch；
* 创建时间；
* 可见性；
* 小修改标志。

```text
Page 1 ── N Revision
Revision N ── 1 ContentSnapshot
```

首版使用单父版本：

```text
revision.parent_revision_id
```

多人编辑和 AI Proposal 在发布前完成合并，最终进入一条正式版本主线。

## 4.7 ContentSnapshot

ContentSnapshot 保存某次 Revision 的完整 Typed Block AST。

保存内容包括：

* AST Schema 版本；
* 完整 AST JSON；
* 内容哈希；
* 内容大小；
* 创建时间。

ContentSnapshot 不保存页面标题、编辑者或修改摘要，这些属于 Page 和 Revision。

## 4.8 WorkingDocument

WorkingDocument 表示页面当前正在编辑的工作副本。

主要用于：

* 多人实时协作；
* 自动保存；
* 离线编辑合并；
* 光标和在线状态；
* AI 建议在编辑器中的预览。

```text
WorkingDocument
        ↓ publish
ContentSnapshot
        ↓
Revision
```

WorkingDocument 可以使用 CRDT，但 CRDT 状态不是正式版本历史。

## 4.9 Block

Block 是页面中的稳定结构单元。

常见 Block 类型：

```text
Heading
Paragraph
BulletList
OrderedList
ListItem
Table
TableRow
TableCell
Image
Video
Quote
Code
Callout
Component
DatasetView
Embed
Divider
```

每个需要独立移动、评论、引用或更新的 Block 都应有稳定 ID。

Block ID 用于：

* AI 精确修改；
* 块级 Diff；
* 块级评论；
* 章节定位；
* 并发冲突检测；
* 来源绑定；
* 块迁移和重定向。

---

# 5. Typed Block AST

页面正文使用树形 AST，而不是 Wikitext 字符串。

```text
Document
├── HeadingBlock
├── ParagraphBlock
│   └── Inline Nodes
├── ListBlock
│   └── ListItemBlock
├── TableBlock
├── ComponentBlock
└── DatasetViewBlock
```

行内节点包括：

```text
Text
PageReference
PageAnchorReference
EntityReference
ExternalLink
CitationReference
ClaimReference
InlineCode
Math
Mention
```

## 5.1 页面引用

已解析页面引用保存：

```text
target_page_id
optional target_heading_block_id
display_text
```

页面改名不需要修改正文中的引用。

## 5.2 未解析页面引用

目标页面不存在时，AST 可以保存：

```text
target_namespace
normalized_title
expected_entity_type
resolution_status
```

后台 Resolver 在新页面创建后尝试自动解析。

## 5.3 实体引用

正文中的人物、地点、组织和作品使用 `EntityReference`。

它表示：

```text
正文提到了某个 Entity
```

但不自动等价于一个 Claim。

## 5.4 Claim 引用

重要结构化事实可以使用 `ClaimReference`。

例如页面中的发布日期可以直接引用 Claim，而不是复制固定文本。

Claim 变化后，系统可以根据 `claim_usage` 找到受影响页面并重新渲染。

## 5.5 Citation 引用

CitationReference 表示某个段落、句子或 Claim 由具体来源支持。

支持粒度：

```text
Block 级
句子或 Inline Node 级
Claim 级
```

---

# 6. Entity 与知识模型

## 6.1 Entity

Entity 表示现实世界或虚构世界中的稳定对象。

例如：

```text
Person
Organization
Place
Work
Character
Event
Product
Concept
Species
Software
```

Entity 与 Page 不是一一对应关系。

```text
一个 Entity
├── 中文 Page
├── 英文 Page
├── 日文 Page
└── 专题 Page

一个 Page
├── 描述一个主 Entity
└── 提及多个其他 Entity
```

## 6.2 EntityLabel 与 EntityAlias

EntityLabel 保存：

* 某语言的主要名称；
* 简短描述；
* 是否首选标签。

EntityAlias 保存：

* 别名；
* 历史名称；
* 缩写；
* 导入名称；
* 其他语言变体。

## 6.3 Property

Property 定义 Claim 的谓词及值类型。

例如：

```text
instanceOf
developer
releaseDate
voiceActor
locatedIn
partOf
author
manufacturer
```

Property 可以声明：

* 值类型；
* 主体 Entity 类型；
* 目标 Entity 类型；
* 是否允许多值；
* 唯一性规则；
* 校验 Schema。

## 6.4 Claim

Claim 表示一个独立的结构化事实。

```text
Subject
+ Property
+ Value
+ Qualifier
+ Valid Time
+ Source
+ Verification Status
```

Claim 支持：

* 字符串值；
* 数字值；
* 日期值；
* Entity 值；
* 坐标值；
* 复合值；
* 多值；
* 冲突值；
* 时间有效性；
* 优先级；
* 来源；
* 人工验证状态。

Claim 不应全部塞进 `entity.fields_jsonb`，因为它需要独立版本、来源和冲突管理。

## 6.5 Claim 状态

建议状态分离为两个维度。

业务状态：

```text
proposed
published
rejected
superseded
deprecated
```

验证状态：

```text
unverified
ai_checked
human_verified
disputed
```

## 6.6 Entity 合并

重复 Entity 合并时：

* 保留目标 Entity；
* 旧 Entity 标记为 merged；
* 维护 EntityMerge 映射；
* 原有引用继续可解析；
* 异步生成引用修复 Proposal；
* Claim 和标签按规则迁移。

---

# 7. Source 与证据模型

## 7.1 ExternalResource

ExternalResource 表示一个规范化外部资源。

保存：

* 原始 URL；
* 规范化 URL；
* Canonical URL；
* 域名；
* 路径；
* HTTP 状态；
* 内容哈希；
* 最近检查时间；
* 最近成功时间。

普通外链和引用来源可以共享同一个 ExternalResource。

## 7.2 Source

Source 表示一个可以被导入或引用的完整来源。

例如：

```text
网页
PDF
书籍
图片
视频
API
数据库记录
```

Source 是逻辑来源，不等同于某次抓取结果。

## 7.3 SourceVersion

SourceVersion 表示来源在某个时间点的具体版本。

例如：

```text
网页在 2026-07-01 的抓取快照
PDF 文件的第 3 个版本
API 某次返回结果
```

一个 Source 可以拥有多个 SourceVersion。

## 7.4 SourceChunk

SourceVersion 被拆分为可定位和可检索的 Chunk。

保存：

* 顺序；
* 页码；
* 章节；
* 文本范围；
* 图片区域坐标；
* OCR 区域；
* 文本内容；
* 内容哈希。

## 7.5 Citation

Citation 指向 SourceVersion 中的特定位置。

```text
SourceVersion
+ SourceChunk
+ Locator
+ Optional Quotation
```

Citation 可以支持：

* 页面段落；
* 具体句子；
* Claim；
* 信息框字段；
* AI 抽取结果。

---

# 8. 外部链接模型

普通 External Link 与 Citation 必须区分。

```text
ExternalLink
= 用户点击跳转的链接

Citation
= 支持某段内容或 Claim 的证据
```

页面中的 ExternalLink 引用稳定的 `external_resource_id`。

URL 发生以下变化时，不一定需要修改正文 AST：

```text
HTTP → HTTPS
同域 301/308
去除追踪参数
Canonical URL 变化
```

只需更新 ExternalResource。

目标完全变更时，生成 `RetargetExternalLink` Proposal。

---

# 9. 页面章节与锚点

Heading Block 同时承担章节身份。

系统使用：

```text
page_id + heading_block_id
```

作为真实章节定位。

页面 URL 可以显示为：

```text
/page-title#section-slug
```

但 slug 不是权威身份。

章节改名时：

* Heading Block ID 不变；
* 更新 PageAnchor.current_slug；
* 旧 slug 写入 PageAnchorAlias。

章节移动到其他页面时，通过 BlockRedirect 保存旧地址映射。

---

# 10. Collection、分类与数据集

## 10.1 Collection

Collection 表示 Page 或 Entity 的集合。

支持：

```text
Manual Collection
Rule Collection
Dynamic Collection
```

Manual Collection 由用户或 AI 明确添加成员。

Rule Collection 根据 Claim 或类型规则生成。

Dynamic Collection 根据查询实时或异步计算。

## 10.2 Dataset

需要筛选、排序和聚合的数据不应只存在普通 Table Block 中。

```text
排版表格
→ Table Block AST

可查询数据表
→ Dataset + DatasetRecord + DatasetView
```

页面中只保存 DatasetView Block。

---

# 11. Component 与信息框

信息框是组件，不是正文中的字段副本。

```text
ComponentBlock
├── component_id
├── component_version
├── entity_id
└── display_config
```

信息框根据 Entity 和 Claim 渲染。

组件版本或 Claim 更新后，通过：

```text
component_dependency
claim_usage
```

找到需要重新渲染的页面。

---

# 12. 多人实时协作

## 12.1 WorkingDocument 与 CRDT

多人同时编辑页面时，共享 WorkingDocument。

CRDT 负责合并：

* 字符插入和删除；
* Block 新增和删除；
* Block 移动；
* Block 属性修改；
* 离线操作；
* 协作光标。

```text
用户 A ─┐
用户 B ─┼── WorkingDocument / CRDT
用户 C ─┘
```

## 12.2 发布与乐观锁

正式发布必须检查基础 Revision。

```text
expected_revision_id
必须等于
page.current_revision_id
```

如果不一致，说明已有其他发布，需要重新合并。

## 12.3 冲突类型

CRDT 可以解决操作层冲突，但不能解决语义冲突。

需要显式处理：

```text
同一 Block 被修改
目标 Block 已删除
同一 Claim 出现不同值
页面被重命名
Entity 被合并
权限发生变化
AI 基于旧版本修改
```

## 12.4 人工和 AI 协作

人工可以直接编辑 WorkingDocument。

AI 默认不作为持续流式输入的实时协作者，而是：

```text
读取 Base Revision
生成 Proposal
执行三方合并
无冲突则合并
有冲突则创建 MergeConflict
```

AI 只在用户主动请求“修改当前段落”时，才可以将一次修改作为原子操作写入 WorkingDocument。

---

# 13. Proposal 与变更模型

## 13.1 Proposal

Proposal 表示尚未正式生效的修改建议。

Proposal 可以针对：

```text
Page
Entity
Claim
Collection
ExternalResource
```

## 13.2 ProposalOperation

Proposal 包含有序的原子操作。

典型操作：

```text
CreatePage
RenamePage
CreateRedirect

InsertBlock
DeleteBlock
MoveBlock
ReplaceBlockContent

InsertPageReference
RetargetPageReference
InsertEntityReference
RetargetEntityReference
InsertCitation
RetargetExternalLink

CreateEntity
MergeEntity
CreateClaim
SupersedeClaim
AddClaimSource

AddCollectionMembership
RemoveCollectionMembership
```

每个 Operation 应包含：

```text
目标 ID
基础 Revision
预期旧值或旧哈希
新值
来源证据
风险信息
```

## 13.3 ReviewTask

ReviewTask 负责人工或策略审核。

低风险修改可以自动通过：

```text
格式规范化
URL Canonical 更新
搜索投影更新
无歧义的页面链接解析
```

高风险修改必须人工审核：

```text
覆盖人工验证 Claim
删除大段正文
更换关键来源
跨域替换外部链接
Entity 合并
批量重命名
```

## 13.4 ChangeBatch

一次 AI 导入或批量操作可以修改多个 Page 和 Claim。

ChangeBatch 用于：

* 批量审计；
* 批量回滚；
* 统计；
* 灰度发布；
* 关联 ImportJob。

## 13.5 MergeConflict

MergeConflict 保存不能自动解决的冲突。

保存：

* Base；
* Current；
* Proposed；
* 冲突类型；
* 目标 Block 或 Claim；
* 解决状态；
* 解决者；
* 最终决议。

---

# 14. AI 自动导入与持续更新

完整流程如下：

```text
上传或抓取 Source
        ↓
创建 SourceVersion
        ↓
解析、OCR、正文抽取
        ↓
生成 SourceChunk
        ↓
实体识别与实体消歧
        ↓
抽取 Entity / Claim / Citation 候选
        ↓
匹配已有 Page、Entity 和 Claim
        ↓
生成 ProposalOperations
        ↓
冲突检测与风险评估
        ↓
自动通过或人工审核
        ↓
创建 Revision / Claim
        ↓
生成 OutboxEvent
        ↓
Projection Builder
        ↓
Render / Search / Cache 更新
```

AI 自动更新可以覆盖：

* 创建页面；
* 修改已有段落；
* 新增或删除段落；
* 修改段落中的页面链接；
* 修改章节跳转；
* 新增实体引用；
* 更新外部链接；
* 增加 Citation；
* 创建或替代 Claim；
* 更新分类；
* 更新信息框依赖。

正文修改必须创建新 Revision。

仅更新投影或 URL 状态时，不创建页面 Revision。

---

# 15. Projection 与自动重建

正式 Revision 发布后，由 Projection Builder 从当前 AST 生成：

```text
PageLinkProjection
EntityMentionProjection
ExternalLinkUsage
CitationUsage
ClaimUsage
ComponentDependency
DocumentOutlineProjection
CollectionMembership
RenderedPage
SearchDocument
```

每个投影必须记录来源 Revision。

```text
projection.source_revision_id
```

异步任务更新投影时，必须确认该 Revision 仍是页面当前 Revision，防止旧任务覆盖新投影。

---

# 16. 事件与最终一致性

发布事务中同步写入：

```text
Revision
Page.current_revision_id
OutboxEvent
AuditEvent
```

异步 Worker 处理：

```text
解析 AST
更新 Projection
渲染 HTML
更新搜索索引
刷新缓存
发送通知
```

Projection 更新失败不能回滚已成功发布的 Revision，但必须：

* 记录失败状态；
* 支持重试；
* 支持按 Page 重建；
* 暴露投影延迟和积压指标。

---

# 17. 阅读、编辑与搜索路径

## 17.1 阅读路径

```text
CDN
→ RenderedPage Cache
→ RenderedPage Storage
→ PostgreSQL ContentSnapshot
```

阅读页不挂载完整编辑器。

## 17.2 编辑路径

```text
WorkingDocument
→ CRDT
→ Editor
→ Publish
→ ContentSnapshot
```

## 17.3 关系查询

以下查询必须走投影表：

* 反向链接；
* 某实体在哪些页面被提及；
* 某来源被哪些页面使用；
* 某组件被哪些页面引用；
* 某 Claim 被哪些页面展示；
* 某分类有哪些成员。

不能在查询时扫描所有 AST JSON。

## 17.4 全文搜索

全文和混合搜索由独立搜索引擎负责：

```text
关键词搜索
实体搜索
字段过滤
高亮
聚合
语义检索
```

PostgreSQL 保存搜索权威源数据，不承担主要全文检索流量。

---

# 18. 数据库表设计

## 18.1 站点与页面

### `wiki_site`

```text
id
site_key
name
default_language
settings_json
created_at
```

### `namespace`

```text
id
wiki_id
namespace_key
canonical_name
display_name
is_content
created_at
```

唯一约束：

```text
wiki_id + namespace_key
```

### `page`

```text
id
wiki_id
namespace_id
normalized_title
display_title
language
content_model
status
current_revision_id
primary_entity_id
created_by
created_at
updated_at
deleted_at
```

唯一索引：

```text
wiki_id
+ namespace_id
+ normalized_title
```

### `page_alias`

```text
id
wiki_id
namespace_id
normalized_title
page_id
alias_type
created_at
```

索引：

```text
wiki_id + namespace_id + normalized_title
page_id
```

### `page_redirect`

```text
source_page_id
target_page_id
target_namespace_id
target_title
target_anchor_block_id
target_interwiki
created_at
```

### `page_entity_binding`

```text
page_id
entity_id
binding_role
language
created_at
```

### `page_anchor`

```text
page_id
revision_id
heading_block_id
parent_heading_block_id
level
title
current_slug
position_key
```

### `page_anchor_alias`

```text
page_id
old_slug
heading_block_id
created_at
```

### `block_redirect`

```text
old_page_id
old_block_id
new_page_id
new_block_id
created_at
```

---

## 18.2 Revision 与内容

### `revision`

```text
id
page_id
parent_revision_id
content_snapshot_id
actor_id
change_batch_id
summary
is_minor
visibility
created_at
```

主要索引：

```text
page_id + created_at DESC + id DESC
actor_id + created_at DESC
change_batch_id
```

### `content_snapshot`

```text
id
schema_version
ast_json
content_hash
size_bytes
created_at
```

索引：

```text
content_hash + schema_version
```

AST 不建议默认建立通用 GIN 索引。

### `working_document`

```text
page_id
base_revision_id
crdt_snapshot
state_version
updated_at
```

### `working_document_update`

```text
page_id
sequence
actor_id
update_data
created_at
```

主键或唯一约束：

```text
page_id + sequence
```

---

## 18.3 Actor 与权限

### `actor`

```text
id
actor_type
user_id
display_name
external_key
status
created_at
```

`actor_type`：

```text
human
anonymous
bot
ai
import
system
```

### `role`

```text
id
role_key
name
description
```

### `actor_role`

```text
actor_id
role_id
wiki_id
created_at
```

### `page_protection`

```text
id
page_id
action_type
required_role_id
expires_at
created_by
created_at
```

---

## 18.4 Entity 与 Claim

### `entity_type`

```text
id
type_key
name
schema_json
created_at
```

### `entity`

```text
id
wiki_id
entity_type_id
canonical_key
status
merged_into_entity_id
created_by
created_at
updated_at
```

### `entity_label`

```text
entity_id
language
label
description
is_primary
```

### `entity_alias`

```text
id
entity_id
language
alias
normalized_alias
alias_type
created_at
```

索引：

```text
language + normalized_alias
entity_id
```

### `property`

```text
id
property_key
name
value_type
subject_type_id
target_type_id
is_multivalued
schema_json
created_at
```

### `claim`

```text
id
subject_entity_id
property_id
value_type
value_json
target_entity_id
qualifiers_json
rank
status
verification_status
valid_from
valid_to
origin_type
change_batch_id
created_by
created_at
superseded_by
```

主要索引：

```text
subject_entity_id + property_id + status
target_entity_id + property_id
change_batch_id
verification_status + status
```

### `claim_source`

```text
claim_id
citation_id
support_type
created_at
```

`support_type`：

```text
supports
contradicts
context
```

---

## 18.5 来源、引用与媒体

### `external_resource`

```text
id
original_url
normalized_url
canonical_url
domain
path
http_status
content_hash
status
redirect_target_id
last_checked_at
last_success_at
created_at
updated_at
```

索引：

```text
normalized_url
canonical_url
domain
status + last_checked_at
```

### `source`

```text
id
source_type
external_resource_id
asset_id
title
author
publisher
published_at
metadata_json
created_at
```

### `source_version`

```text
id
source_id
version_hash
raw_asset_id
extracted_asset_id
fetched_at
created_at
```

唯一约束：

```text
source_id + version_hash
```

### `source_chunk`

```text
id
source_version_id
ordinal
locator_json
text_content
text_hash
created_at
```

索引：

```text
source_version_id + ordinal
text_hash
```

### `citation`

```text
id
source_version_id
source_chunk_id
locator_json
quotation
quotation_hash
created_by
created_at
```

### `asset`

```text
id
wiki_id
name
current_revision_id
status
created_at
updated_at
```

### `asset_revision`

```text
id
asset_id
storage_key
content_hash
mime_type
size_bytes
width
height
metadata_json
actor_id
created_at
```

---

## 18.6 Collection 与 Dataset

### `collection`

```text
id
wiki_id
collection_type
title
description_page_id
query_json
created_by
created_at
updated_at
```

### `collection_membership`

```text
collection_id
page_id
entity_id
member_type
source_type
sort_key
source_revision_id
created_at
```

主要索引：

```text
collection_id + sort_key
page_id
entity_id
```

### `dataset`

```text
id
wiki_id
name
schema_json
created_by
created_at
```

### `dataset_record`

```text
id
dataset_id
entity_id
values_json
created_at
updated_at
```

### `dataset_view`

```text
id
dataset_id
view_type
config_json
created_at
```

---

## 18.7 Component

### `component`

```text
id
component_key
name
created_at
```

### `component_version`

```text
component_id
version
props_schema_json
renderer_ref
status
created_at
```

唯一约束：

```text
component_id + version
```

---

## 18.8 Proposal、审核与冲突

### `import_job`

```text
id
job_type
status
initiated_by
config_json
created_at
started_at
finished_at
error_json
```

### `extraction_run`

```text
id
import_job_id
source_version_id
model_id
prompt_version
schema_version
deduplication_key
status
output_json
created_at
finished_at
```

唯一约束：

```text
deduplication_key
```

### `proposal`

```text
id
import_job_id
target_type
target_id
base_revision_id
base_state_version
status
risk_level
created_by
created_at
updated_at
```

主要索引：

```text
status + risk_level + created_at
target_type + target_id
import_job_id
```

### `proposal_operation`

```text
id
proposal_id
sequence
operation_type
target_page_id
target_block_id
target_node_id
target_entity_id
target_claim_id
expected_hash
payload_json
created_at
```

### `review_task`

```text
id
proposal_id
status
reviewer_id
decision_reason
created_at
reviewed_at
```

### `merge_conflict`

```text
id
proposal_id
page_id
conflict_type
target_block_id
target_claim_id
base_revision_id
current_revision_id
base_value_json
current_value_json
proposed_value_json
status
resolved_by
resolution_json
created_at
resolved_at
```

### `change_batch`

```text
id
import_job_id
proposal_id
actor_id
status
created_at
rolled_back_at
```

---

## 18.9 页面关系投影

### `page_link_projection`

```text
source_page_id
source_revision_id
source_block_id
source_node_id
target_page_id
target_namespace_id
target_title
target_anchor_block_id
resolution_status
display_text
```

主要索引：

```text
target_page_id + source_page_id
target_namespace_id + target_title
source_page_id + source_revision_id
```

### `entity_mention_projection`

```text
page_id
revision_id
block_id
node_id
entity_id
mention_text
```

索引：

```text
entity_id + page_id
page_id + revision_id
```

### `external_link_usage`

```text
external_resource_id
page_id
revision_id
block_id
node_id
link_role
```

索引：

```text
external_resource_id + page_id
page_id + revision_id
```

### `citation_usage`

```text
citation_id
page_id
revision_id
block_id
node_id
claim_id
```

### `claim_usage`

```text
claim_id
page_id
revision_id
block_id
node_id
```

### `component_dependency`

```text
component_id
component_version
page_id
revision_id
block_id
entity_id
```

### `document_outline_projection`

```text
page_id
revision_id
heading_block_id
parent_heading_block_id
level
title
position_key
```

所有页面投影都必须带 `revision_id`。

---

## 18.10 事件、审计和渲染

### `audit_event`

```text
id
actor_id
event_type
aggregate_type
aggregate_id
change_batch_id
payload_json
created_at
```

### `change_tag`

```text
id
tag_key
name
description
created_at
```

### `change_tag_assignment`

```text
tag_id
revision_id
proposal_id
audit_event_id
```

### `outbox_event`

```text
id
aggregate_type
aggregate_id
event_type
payload_json
status
attempt_count
created_at
processed_at
last_error
```

### `projection_state`

```text
aggregate_type
aggregate_id
projection_type
source_version_id
status
projected_at
last_error
```

### `rendered_page`

```text
page_id
revision_id
renderer_version
html_content
content_address
content_hash
created_at
```

---

# 19. 首版范围

## P0：系统地基

必须实现：

```text
WikiSite / Namespace
Page / PageAlias / PageRedirect
Revision / ContentSnapshot
Typed Block AST
Actor
Entity / Property / Claim
Source / SourceVersion / SourceChunk / Citation
Proposal / ProposalOperation / ReviewTask
ImportJob / ChangeBatch
PageLinkProjection
EntityMentionProjection
ExternalResource / ExternalLinkUsage
CitationUsage
OutboxEvent / ProjectionState
AuditEvent
RenderedPage
```

## P1：协作与自动化增强

```text
WorkingDocument / CRDT
MergeConflict
PageAnchorAlias
BlockRedirect
ClaimUsage
Component / ComponentDependency
Collection
自动外链检查
实体合并
批量风险审核
```

## P2：规模化与高级能力

```text
Dynamic Collection
Dataset
高级 Claim Qualifier
AI 信任等级
自动事实一致性检查
语义搜索和图查询
跨 Wiki Entity Federation
章节级懒加载
冷热 Revision 分层存储
```

---

# 20. 必须长期保持的系统不变量

1. `Page.current_revision_id` 必须指向该 Page 的有效 Revision。
2. 已发布 Revision 和 ContentSnapshot 不可修改。
3. Projection 必须能够从权威数据重建。
4. Projection 必须标记其对应的 Revision 或 Claim 版本。
5. LLM 不直接更新 Projection 或正式 Revision。
6. 页面改名不改变 Page ID。
7. 章节改名不改变 Heading Block ID。
8. Entity 改名或合并必须保留旧 ID 的解析路径。
9. Claim 必须允许绑定来源、验证状态和时间有效性。
10. AI 修改人工验证内容必须经过风险策略或审核。
11. 正文修改必须产生新 Revision。
12. 外部资源状态变化不一定产生页面 Revision。
13. 人工和 AI 并发修改必须基于 Base 版本执行三方合并。
14. 批量 AI 操作必须能够按 ChangeBatch 审计和回滚。

---

# 21. 最终系统定位

本系统可以概括为：

```text
Notion / 思源式结构化块文档
+
Wikipedia 式页面、版本和治理
+
Wikidata 式 Entity / Claim
+
Git 式 Patch、Review 和 Merge
+
CRDT 式多人实时协作
+
AI 多模态导入与持续更新
```

系统中的职责边界为：

```text
编辑器
负责产生结构化文档操作

LLM
负责理解内容、抽取知识和提出变更

领域服务
负责校验、权限、消歧、冲突和合并

Revision / Claim
负责保存正式权威状态

Projection Builder
负责生成链接、目录、引用和依赖

Renderer
负责生成阅读页面

Search
负责全文、实体和语义检索

Governance
负责审核、保护、审计和回滚
```

这套设计能够同时支持：

* 人工编辑；
* 多人协作；
* AI 修改已有内容；
* 自动创建页面；
* 页面与章节链接稳定；
* 外部链接持续维护；
* Entity 和 Claim 自动更新；
* 来源级事实溯源；
* 信息框自动渲染；
* 批量审核与回滚；
* 中型 Wiki 的常用查询和读取负载。
