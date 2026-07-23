# knowledge — Entity（M4-T02）+ Claim（M4-T03）领域服务

Entity 身份与标签/别名/页面绑定、Property/Claim/ClaimSource 生命周期的**唯一权威写入入口**。
风格与边界同 `internal/page`：
手写 SQL（ADR-0002）、Querier 模式、哨兵错误 + `%w` 包装、跨表写入走 `db.TxManager` 单事务。
M4-T08 的 HTTP 层仅调用本 Service 的只读方法，不绕过领域边界：
`GET /api/v1/entities/{id}` 与 `GET /api/v1/claims/{id}`；前端详情页同时读取
M4-T07 使用投影，保持权威详情与可重建关系查询分离。

## 模型

| 类型 | 表 | 说明 |
|---|---|---|
| `Entity` | `entity` | 稳定身份。`canonical_key` 每 wiki 唯一（规范化：NFC + 折叠空白 + 小写，复用 `page.NormalizeTitle`）；`status`: active/merged/deleted |
| `EntityLabel` | `entity_label` | 多语言标签（PK = entity+language+label）。落库形态 NFC + trim + 折叠空白（保留大小写）；同 entity+language 至多一个 `is_primary`，DB 部分唯一索引 `entity_label_primary_key` 兜底 |
| `EntityAlias` | `entity_alias` | 别名。`alias` 保留书写，`normalized_alias` 按标题同规则规范化；同实体规范化重复拒绝（服务层在实体行锁内前置检查） |
| `EntityType` | `entity_type` | 000004 固定 UUID 种子（person/organization/…/software），按 `type_key` 解析 |
| `PageEntityBinding` | `page_entity_binding` | 页面-实体绑定，role ∈ primary/mentioned（PK = page+entity+role） |

不变量：实体始终保有 ≥1 个主标签（CreateEntity 校验；RemoveLabel 拒绝删除仅剩的主标签）。

## 搜索语义（`SearchEntities`）

M7 全文检索落地前的权威数据直连实现（不引入 pg_trgm）：

1. 查询串先按标题同规则规范化；
2. **exact 阶段**：`canonical_key` 相等 → 标签规范化相等（`lower(label)`，标签落库已 NFC + 折叠空白，严格可比）→ `normalized_alias` 相等；
3. **fuzzy 阶段**：上述三者的 ILIKE 前缀/包含（`%`/`_`/`\` 已转义）。

每个实体只返回一次，结果带 `MatchedOn`（canonical/label/alias）与 `Exact` 标记；
排序 = 阶段顺序：exact 优先，同阶段内 canonical → label → alias。
`TypeKey` 可选过滤；`Limit` 缺省 20、上限 100；默认仅 active 实体，
`IncludeMerged` 时含 merged（deleted 永远排除）。

## 合并边界

- **同名不自动合并**（MT-M4-NO-AUTOMERGE）：创建只按 `canonical_key` 判重；
  标签/别名相同的已有实体不影响创建，搜索返回全部候选（`TestSearchEntities_NoAutoMerge`）。
- **merged 实体**：全部写操作（标签/别名/绑定）拒绝 `ErrEntityMerged`；搜索默认排除。
  合并功能本身（状态流转、引用修复、EntityMerge 映射）属 M9-T06。

## Actor 准入

`CreateEntity` 复用 page 包的 Actor 校验（`page.Repository.CheckWriteActor`，规则单点维护）：
human/bot/system 可写，ai 拒绝（`page.ErrActorNotAllowed`），不存在/停用报 `page.ErrInvalidActor`。

## 已知边界（后续 Task）

- `page.primary_entity_id` 指针同步不在本 Task：BindPage 只写 `page_entity_binding`。
- 标签/别名/绑定写操作暂不校验 Actor（领域服务签名按 Task Packet；API 层落地时统一鉴权）。
- PG `lower()` 与 Go `strings.ToLower` 在极少数 Unicode 大小写规则上有差异，exact 标签匹配以 PG 为准。

---

# Claim 领域服务（M4-T03）

`claim.go` / `claim_types.go` / `claim_repository.go`：Property/Claim/ClaimSource 的权威写入。
与 Entity 服务共用 `Service` 装配与事务/行锁约定。

## 模型

| 类型 | 表 | 说明 |
|---|---|---|
| `Property` | `property` | 谓词定义（000004 固定 UUID 种子 8 个）。`value_type` ∈ string/number/date/entity/coordinate/composite；`subject_type_id`/`target_type_id` 列约束与 `schema_json.subject_type`/`target_type`（type_key 形态）由服务层并列校验 |
| `Claim` | `claim` | 结构化事实。业务状态与验证状态正交（设计 §6.5）；`superseded_by` 指向链上下一个 claim；有效时间双开区间（DB `claim_valid_time_check` 兜底，服务层先给 `ErrInvalidValidTime`） |
| `ClaimSource` | `claim_source` | claim↔citation 关联（PK = claim+citation）。校验 citation_id、support_type 与 Citation 存在性；DB 外键并发兜底 |

## 值类型形态（value_json）

| value_type | value_json 形态 | 服务层校验 |
|---|---|---|
| `string` | JSON string | 非空串 |
| `number` | JSON number | 拒绝 NaN/Inf |
| `date` | JSON string（RFC3339 date，`YYYY-MM-DD`） | `time.Parse("2006-01-02")` |
| `entity` | `{"entity_id": "<uuid>"}`，冗余 `target_entity_id` 列 | 目标实体存在且 active；类型匹配 `target_type_id`/`schema_json.target_type` |
| `coordinate` | `{"lat": <number>, "lon": <number>}` | lat∈[-90,90]，lon∈[-180,180]，拒绝 NaN/Inf |
| `composite` | 自由 JSON object | 必须是 object；`schema_json.value` 非空时按子集 Schema 校验（`required` 必填键 + `properties.<key>.type` 基本类型） |

入参用 `Value` 包装（`StringValue`/`NumberValue`/`DateValue`/`EntityValue`/`CoordinateValue`/`CompositeValue` 构造），
按 property 的 value_type 取对应字段。`schema_json` 的更完整 Schema 校验是简版实现，
留 TODO 由 M5 完善（见 `claim_types.go` `propertySchema` 注释）。

## 业务状态机（claim.status）

```text
           PublishClaim            RejectClaim
proposed ────────────► published  ◄──────────  （仅 proposed 有出边）
   │                      │
   │                      ├─ DeprecateClaim ──► deprecated（终态）
   │                      └─ SupersedeClaim ──► superseded（终态，仅经 Supersede 链产生）
   └── RejectClaim ──► rejected（终态）
```

- 初始状态：`origin_type=human` → published；`ai`/`import` → proposed
  （M5 治理落地前的人工预放行，治理后经 Proposal 审核收紧）。
- rejected/deprecated/superseded 为终态；公开流转方法从不以 superseded 为目标。
- 单值不变量：`is_multivalued=false` 时同 subject+property 至多一个 published——
  CreateClaim 创建侧拒绝（提示先 Supersede），PublishClaim 发布侧兜底（防御性），
  计数均在 subject 实体行锁内，序列化并发（并发创建恰一成功有集成测试）。
- 状态机全矩阵单测：`TestClaimStatusTransitionMatrix`。

## 验证状态（claim.verification_status，与业务状态正交）

`unverified`（初始）/ `ai_checked` / `human_verified` / `disputed`。
权限矩阵（`checkVerificationPermission`，防御性）：

| Actor 类型 | 可置状态 |
|---|---|
| human | 全部四种 |
| ai | 仅 `ai_checked` |
| bot / system / 其他 | 无权（`ErrVerificationForbidden`） |

## Supersede 语义

`SupersedeClaim` 单事务内：按 CreateClaim 同款校验创建新 claim（值/类型/有效时间/单值跳过自身），
旧 claim 置 `superseded` 且 `superseded_by` 指向新 claim；旧 claim 保留可审计。

- 旧 claim 必须 published（状态机只允许 published→superseded）；
  已 superseded/deprecated/rejected 的再 supersede 报 `ErrInvalidClaimTransition`。
- `SubjectEntityID`/`PropertyKey` 是乐观断言：与旧 claim 实际值不符报
  `ErrClaimSubjectMismatch`（防止基于过期认知替换值）。
- `OriginType` 空时继承旧 claim；新 claim 初始状态仍按 origin 映射。
- 并发：旧 claim 行锁（FOR UPDATE）序列化，并发双 supersede 恰一成功
  （`TestSupersedeClaim_Concurrent`）。
- 写入顺序：先插新 claim 再置旧 claim（`superseded_by` 外键要求被指向行已存在），
  任一步失败整体回滚。

## Actor 准入与审核边界

- `CreateClaim`/`SupersedeClaim` 复用 `page.CheckWriteActor`（human/bot/system 可写，
  ai actor 拒绝）——ai/import 来源的 Claim 由 bot/system 管道 Actor 录入，
  与 page 模块 M5 前立场一致；INV-05/INV-08 的 AI 直改限制在 M5-T06/T10 落地。
- `PublishClaim`/`RejectClaim`/`DeprecateClaim` 暂不校验操作者（M5 审核治理统一加）。
- `UpdateVerificationStatus` 校验 Actor 存在且 active（查不到保守拒绝为 `page.ErrInvalidActor`）。
