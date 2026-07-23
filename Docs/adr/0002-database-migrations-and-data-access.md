# ADR-0002：SQL 迁移与数据访问

状态：已接受（M0-T02）
日期：2026-07-21

## 背景

实施方案要求「Go Repository + 显式 SQL Migration」，复杂事务、约束和索引必须可审查，不引入多数据库抽象。

## 决策

- 迁移工具：**golang-migrate/migrate v4**（CLI + Go library 同源），迁移文件位于 `backend/migrations`，命名 `{seq}_{name}.up.sql` / `{seq}_{name}.down.sql`，序号全局唯一、只增不改。
- 迁移所有权：同一时间只允许一个 Task 新增迁移文件（实施方案 §8.2）；合并冲突时后合入者重排序号。
- 应用启动**不自动执行迁移**；由 `make migrate-up` 或部署流水线的 Migration gate 显式执行。
- 数据访问：**jackc/pgx v5**（`pgxpool`），手写 Repository，SQL 字符串内联在 repository 文件中以便逐行审查。
- 事务：Repository 方法接收 `context.Context` 与 `pgx.Tx` 接口（`platform/db` 定义 `TxManager`），跨表原子写入（如发布事务）由领域服务在一个事务内编排。
- 不引入 ORM、不引入 sqlc：P0 阶段显式 SQL 优先于代码生成，减少一条生成链路。

## 备选方案

- goose：能力相当；golang-migrate 的 dirty-state 处理和 CLI 生态更成熟。
- sqlc：类型安全更好，但增加生成物漂移检查负担；P1 可重新评估。
- GORM：违反「显式 SQL 可审查」原则，排除。

## 影响

- 每个含迁移的 Task 必须提供 up/down 两份 SQL 并在 Task Packet 说明三种场景（空库升级、已有库升级、回滚）。
- CI 增加迁移校验：序号唯一性、up/down 成对、空库可完整 upgrade。

## 预发布修订（2026-07-24）

项目尚未上线且开发数据无需保留，历史迁移已压缩为
`000001_initial_schema.up/down.sql`。首次生产上线前，Schema 变更直接同步修改该
up/down，并验证空库初始化、完整清库和再次初始化；不新增增量版本。

首次生产上线并承载需保留的数据后，本修订自动结束，恢复本 ADR 的只增不改策略。
