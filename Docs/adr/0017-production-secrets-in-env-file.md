# ADR-0017：生产机密统一写入部署环境文件

- 状态：已接受
- 日期：2026-07-27

## 背景

ADR-0016 使用 Compose file secrets 与 `*_FILE` 环境变量注入生产机密。这要求单独
维护 `SECRETS_DIR`、五个机密文件和容器入口转换脚本。当前部署仍处于封闭早期阶段，
操作者选择用一个部署环境文件统一管理配置与机密，以降低准备和更新成本。

## 决策

- `POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD`、`S3_ACCESS_KEY`、`S3_SECRET_KEY`
  直接写入仓库外的
  `DEPLOY_ENV_FILE`。
- `docker compose --env-file` 将这些值注入对应容器环境。
- 删除 Compose `secrets`、`*_FILE` 转换和 `container-entrypoint`。
- `deploy.sh` 在任何生产变更前校验上述变量非空，并拒绝 group/world 可读取的
  部署环境文件。
- 环境文件保持 shell 兼容的 `KEY=VALUE` 格式，不提交到仓库，不输出到日志或工单。
- Compose 从 `POSTGRES_*` 自动生成内部 `DATABASE_URL`，避免重复保存数据库密码。

## 影响

- 部署只需保护和更新一个文件，操作更直接。
- 机密会进入容器环境；具有 Docker 管理权限的人可以通过 `docker inspect` 等方式
  读取。这比文件挂载的暴露面更大，是当前早期阶段明确接受的安全代价。
- 环境文件泄露等同于数据库和对象存储凭据同时泄露，必须使用 `0600`
  权限、最小化宿主机访问，并在泄露后整体轮换。
- 本决策只取代 ADR-0016 第 6 节；账号体系由 ADR-0019 定义。
- AI Provider 密钥的环境变量注入已被 ADR-0021 取代；部署文件只保留 AI 配置加密
  主密钥和 Sidecar 内网令牌。
