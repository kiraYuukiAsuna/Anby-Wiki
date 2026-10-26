# SQL 初始化 Schema

本目录由 `backend/cmd/migrate`（golang-migrate v4）显式执行，应用启动不会自动修改数据库。

当前 release-candidate 阶段允许对不兼容环境做显式空库重建，因此迁移历史仍压缩为
唯一版本：

```text
000001_initial_schema.up.sql
000001_initial_schema.down.sql
```

`up` 一次创建当前完整 Schema、约束、索引、触发器和以下固定种子：

- System Actor。私人 Wiki、7 个 Namespace、默认 ChangeTag、内置 Component 和
  human Actor 由账号注册领域事务创建，不作为全局种子。
- 10 个基础 EntityType 与 14 个基础 Property，以及 Citation 不可变证据身份唯一索引。
- `editor`、`reviewer`、`applier`、`admin` 四个内置 Role。

`down` 删除完整 Schema，仅用于本地开发和尚未承载生产数据的环境。执行会永久删除全部业务数据。

## Release-candidate 规则

- Schema 变更直接修改 `000001_initial_schema.up.sql`，并同步修改完整逆操作 `down`。
- 不新增增量迁移版本，也不兼容旧开发数据库；变更后直接重建本地数据库。
- 初始化脚本必须同时覆盖空库 `up`、完整 `down`、再次 `up`。
- 部署 gate 的 expected/min/max 均固定为 `1`。
- 生产运行中禁止执行 `down`；修改后的 `000001` 不能用于在线升级已有库，不兼容变更
  必须显式重建对应环境。
- 一旦进入数据保留承诺，必须冻结 `000001`，恢复只增不改的增量迁移策略，并更新本
  说明与 `AGENTS.md`。

## 命令

```bash
make pg-reset       # 重建本地 PostgreSQL 并应用初始化 Schema
make migrate-up     # 对已启动的数据库执行初始化
make migrate-down   # 完整删除当前 Schema（仅一个版本）
```
