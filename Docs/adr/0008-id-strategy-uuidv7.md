# ADR-0008：ID 策略（UUIDv7）

状态：已接受（M0-T02）
日期：2026-07-21

## 背景

Page、Revision、Block、Entity、Claim、Source、Citation 等对象需要稳定 ID（设计 §3.2）；需统一生成方式、存储类型与 API 序列化格式。

## 决策

- 格式：**UUIDv7**（时间有序），应用侧生成（Go：`google/uuid` v1.6+ 的 `uuid.NewV7`；前端 Block ID：`uuid` npm 包 v9+）。
- 存储：PostgreSQL `uuid` 原生类型。
- API/JSON 序列化：标准 36 字符小写连字符形式。
- 数据库**不**使用 `gen_random_uuid()` 默认值生成业务 ID：ID 由领域服务在事务内生成，便于构造 Outbox 载荷与幂等键。
- AST 内 Block ID 同样使用 UUIDv7 字符串，由编辑器 Adapter/AI Patch Engine 生成，Schema 中用 `format: uuid` 校验。
- 排序需求（如 Revision 历史）不依赖 ID 时间序，显式使用 `created_at + id` 排序。

## 备选方案

- UUIDv4：随机分布导致 B-tree 插入碎片化，十万级以上 Revision 表不友好。
- ULID/KSUID：字符串排序友好但 PG 无原生类型，生态弱于 UUIDv7。
- 自增 bigint：暴露规模信息且不利于多写入方（编辑器本地生成 Block ID）。

## 影响

- 所有迁移中的主键列统一 `uuid PRIMARY KEY`（无外键语义的特殊列除外）。
- `platform/id` 提供唯一生成入口，测试可注入固定时钟。
