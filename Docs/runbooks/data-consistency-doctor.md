# 数据一致性巡检 Runbook

## 适用范围

`cmd/doctor` 对 PostgreSQL 做一次性巡检，默认只读。适用于发布前门禁、定期巡检、恢复演练后验收和事故排查。

它不替代数据库约束，也不自动修复权威百科数据。Page、Revision、Entity、Claim、Citation 等问题必须通过 Proposal 或对应领域服务处理。

## 快速执行

从 `backend/` 目录运行：

```bash
DATABASE_URL='postgres://...' go run ./cmd/doctor
```

机器可读报告：

```bash
DATABASE_URL='postgres://...' go run ./cmd/doctor -format json > doctor-report.json
```

调整 Outbox claimed 卡死阈值：

```bash
DATABASE_URL='postgres://...' go run ./cmd/doctor -claimed-stuck-after 15m
```

显式清理过期 auth 临时态：

```bash
DATABASE_URL='postgres://...' go run ./cmd/doctor -repair-expired-auth
```

`-repair-expired-auth` 只删除执行时已过期的 `oidc_login_attempt` 和 `auth_session`。它不清理未过期或仅 revoked 的会话，也不触碰权威百科数据。

## 退出码

| 退出码 | 含义 | 值班动作 |
|---|---|---|
| `0` | 无 `error` / `critical` | 可继续门禁；仍应评估 warning |
| `1` | 发现 `error` / `critical` | 阻断发布或恢复验收，按 issue code 处置 |
| `2` | 参数错误 | 修正 flag |
| `3` | 数据库连接、查询或报告写出失败 | 检查运行环境；本次结果不可作为健康证明 |

## 严重级别

| 级别 | 语义 |
|---|---|
| `critical` | 权威内容、不可变保护或核心证据链可能损坏，立即阻断 |
| `error` | 引用、投影或异步处理已不一致，需要处置后重跑 |
| `warning` | 存在可安全清理的临时态或治理质量缺口 |
| `info` | 仅记录，不影响健康退出码 |

## Issue Code

### 文档与不可变性

| Code | 含义 | 处置 |
|---|---|---|
| `DOC_CURRENT_REVISION_MISSING` | current 指向不存在 Revision | 停止写入，经 Page 领域服务/Proposal 恢复 |
| `DOC_CURRENT_REVISION_OWNER_MISMATCH` | current Revision 不属该 Page | 检查发布事务与触发器，经领域服务恢复 |
| `DOC_IMMUTABLE_TRIGGER_MISSING` | 不可变触发器缺失或禁用 | 停止权威写入，恢复已审核 Schema |
| `DOC_CONTENT_INVALID` | ContentSnapshot 无法 canonicalize | 隔离 Revision，发布替代 Revision |
| `DOC_CONTENT_HASH_MISMATCH` | canonical AST 哈希不符 | 隔离 Revision，审计写入链路 |
| `DOC_CONTENT_SIZE_MISMATCH` | canonical AST 大小不符 | 审计发布链路 |
| `EVIDENCE_SOURCE_CHUNK_HASH_MISMATCH` | SourceChunk 文本哈希不符 | 隔离相关 Citation，创建替代证据 |
| `EVIDENCE_CITATION_QUOTATION_HASH_MISMATCH` | 引文与哈希不符 | 创建替代 Citation，经 Proposal 替换 |

### 引用与证据

| Code | 含义 |
|---|---|
| `REF_PAGE_ORPHAN` | Current AST 引用不存在 Page |
| `REF_ENTITY_ORPHAN` | Current AST 引用不存在 Entity |
| `REF_CLAIM_ORPHAN` | Current AST 引用不存在 Claim |
| `REF_CITATION_ORPHAN` | Current AST 引用不存在 Citation |
| `EVIDENCE_CLAIM_SOURCE_ORPHAN` | ClaimSource 的 Claim/Citation 缺失 |
| `EVIDENCE_CITATION_VERSION_ORPHAN` | Citation 的 SourceVersion/Source 缺失 |
| `EVIDENCE_CITATION_CHUNK_VERSION_MISMATCH` | Citation 与 SourceChunk 不属同一版本 |
| `EVIDENCE_PUBLISHED_CLAIM_WITHOUT_SOURCE` | 已发布 Claim 无证据，warning |

权威引用问题只允许经 Proposal 或领域服务修复，不得直接改 AST JSON、ClaimSource 或 Citation。

### 投影与 Search

| Code | 含义 |
|---|---|
| `PROJECTION_STATE_MISSING` | 已发布页面缺少 Builder 状态 |
| `PROJECTION_STATE_ERROR` | Builder 状态为 error |
| `PROJECTION_SOURCE_REVISION_STALE` | 状态来源不是 Current Revision |
| `PROJECTION_ROW_SOURCE_REVISION_STALE` | 投影数据行来源不是 Current Revision |
| `SEARCH_SOURCE_REVISION_STALE` | Search 文档来源不是 Current Revision |

M9-T08 起，页面预期 Builder 集合包含 `component_dependency`；其 state 缺失或数据行
Revision 陈旧使用同一组 `PROJECTION_*` code，并通过页面重建修复。

先确认同一页面没有 `DOC_*` / `REF_*` critical，再使用既有 Worker Rebuilder：

```bash
DATABASE_URL='postgres://...' go run ./cmd/worker -rebuild-page <page-uuid>
DATABASE_URL='postgres://...' go run ./cmd/worker -rebuild-all
```

重建后必须重新运行 doctor。禁止为了消除报告而直接改 `projection_state`。

### Outbox 与 Auth

| Code | 含义 | 处置 |
|---|---|---|
| `OUTBOX_CLAIM_STUCK` | claimed 超过阈值 | 先确认无消费者仍持有，再按 Outbox 恢复流程处理 |
| `OUTBOX_DEAD` | 事件进入 dead | 排查 `last_error`，使用既有 Worker 重放流程 |
| `AUTH_LOGIN_ATTEMPT_EXPIRED` | 过期 OIDC 登录临时态 | 可显式运行 `-repair-expired-auth` |
| `AUTH_SESSION_EXPIRED` | 过期服务端会话 | 可显式运行 `-repair-expired-auth` |

doctor 不自动重放 dead 事件，也不自动释放 claimed 事件。

## JSON 稳定性

- 报告 `version` 当前为 `m9-t08-v1`。
- 自动化应按 `code` 和 `severity` 判断，不应解析中文 `message`。
- Issues 按 code、resource type、resource ID 稳定排序。
- 报告不包含 AST、来源全文、Token、登录 secret、`last_error` 或个人敏感信息。

## 已知 Blocker

- 资产对象字节哈希需要只读访问对象存储并流式计算，本命令只连接 PostgreSQL，暂不验证 `asset_revision.content_hash` 对应的真实对象内容。
- 权威问题没有 doctor 自动修复路径；在 Proposal/领域服务运维流程完成前，所有 critical issue 保持发布 blocker。
