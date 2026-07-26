# scripts 目录

日常开发和质量检查优先使用仓库根的 `make <target>`。运行 `make help` 可查看分组后的公共入口。

本目录只保存 Makefile 背后的实现脚本，以及必须由操作者显式执行的生产运维脚本：

| 路径 | 职责 |
| --- | --- |
| `dev.sh` | 读取根 `.env`，探测依赖并启动 API、Worker、Web |
| `dev-pg.sh` | 管理免 Docker 的本地 PostgreSQL |
| `dev-probe.go` | `dev.sh` 使用的连接探测辅助程序 |
| `check-*.sh` | 契约、迁移和生产部署静态检查 |
| `perf-db.sh` | 重建独立性能测试数据库 |
| `deploy.sh` | 生产部署、迁移、诊断和回滚入口 |
| `backup/` | PostgreSQL 与对象存储的备份/恢复演练 |
| `tests/` | Shell 脚本的无外部服务回归测试 |

`deploy.sh` 与 `backup/` 下的脚本可能修改外部环境，因此不包装成日常 Make 目标。请按
`Deploy.md` 和 `Docs/runbooks/` 中的步骤设置确认令牌后显式执行。
