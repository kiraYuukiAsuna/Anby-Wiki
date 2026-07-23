-- infra/local/postgres/init.sql
-- 本地开发数据库的幂等初始化脚本。
--
-- 注意：此文件挂载到 /docker-entrypoint-initdb.d/，PostgreSQL 仅在数据卷为空
-- （即首次创建容器时）执行一次。脚本本身按幂等写法编写（IF NOT EXISTS），
-- 手动重复执行 `psql -f` 也是安全的。
--
-- 职责边界：
--   本脚本只负责数据库级的基础能力（扩展）。
--   应用 schema 与全部业务表（含 outbox_event，见 ADR-0003）由 Go 迁移工具创建
--   与演进（见根 Makefile 的 migrate-up），此处不建任何业务表，避免与迁移产生
--   两套事实来源。

-- pgcrypto：为后续 UUIDv7 / 哈希等需求提供 gen_random_bytes 等基础函数
-- （ID 策略见 ADR-0008）。
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 应用 schema 说明：
-- 业务表默认建在迁移工具管理的 schema（通常为 public 或迁移指定的 schema）下。
-- 若后续决定使用独立 schema（如 wiki），应在 Go 迁移的第一步中
-- CREATE SCHEMA IF NOT EXISTS，而不是写在这里，以保证本地与 CI / 生产一致。
