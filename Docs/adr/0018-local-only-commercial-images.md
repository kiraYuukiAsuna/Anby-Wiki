# ADR-0018：商业业务镜像仅在部署机本地构建和使用

- 状态：已接受
- 日期：2026-07-27

## 背景

既有流程要求把 API、Worker、Web 和迁移工具镜像推送到 registry，再通过 digest
部署。Anby Wiki 是商业软件，当前交付模式不允许发布自有业务镜像；部署主机持有
受保护源码，并负责本地构建和运行。

## 决策

- Production Compose 为 `api`、`worker`、`web`、`migrate` 声明本地 `build`。
- 本地镜像固定命名为 `anby-wiki-<target>:<RELEASE_ID>`。
- 自有业务服务设置 `pull_policy: never`，部署脚本不执行 registry pull 或 push。
- `deploy` 在迁移和滚动替换前从当前源码构建四个业务镜像。
- `rollback` 不重建源码，只允许切换到部署机上仍然存在的旧版本本地镜像。
- PostgreSQL、Redis、MinIO、Alpine 及语言构建基础镜像仍可从各自上游拉取。

## 影响

- 自有业务镜像不会离开部署机，不依赖私有或公共业务 registry。
- 部署机必须保存完整、受访问控制的商业源码和 Docker 构建上下文，并承担构建时间、
  磁盘空间与基础镜像供应链管理。
- `RELEASE_ID` 成为本地镜像版本边界，必须唯一且不可拿当前源码复用旧版本标签。
- 要保留回滚能力，清理本地镜像时必须保留仍在回滚窗口内的四个旧版本镜像。
