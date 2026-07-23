# PostgreSQL 与对象存储备份恢复 Runbook

## 安全边界

- 恢复只允许显式 `RESTORE_ENVIRONMENT=drill`，并拒绝 `ENV`、`APP_ENV` 或 `ANBY_ENV` 的 production/prod 值。
- PostgreSQL 目标必须是不存在的 `wiki_restore_[a-z0-9_]+`；脚本从不覆盖已有库，也不对已有库执行 `dropdb`。
- 对象目标必须是不存在、与源不同且名称含 `-restore-` 的 bucket。
- 确认令牌绑定目标名称，防止复制旧命令后误操作其他资源。
- 凭据只通过 `PGPASSWORD`/`.pgpass`、`AWS_*` 或 `S3_*` 环境注入。禁止 `set -x`，禁止把含 secret 的命令或环境写入工单。
- PostgreSQL 备份包含 auth/session 等敏感状态。备份目录必须使用加密存储、最小权限和保留期策略；本脚本的 `umask 077` 不是静态加密替代品。

## PostgreSQL 备份

要求客户端主版本与服务端兼容，并在整个 dump 和权威快照期间停止应用写入。`BACKUP_WRITE_QUIESCED=YES` 是人工确认，不会代替流量切断。

```bash
export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"
export PGHOST=127.0.0.1
export PGPORT=55432
export PGUSER=wiki
export SOURCE_DB_NAME=wiki
export BACKUP_DIR=/secure/backups/anby-pg-$(date -u +%Y%m%dT%H%M%SZ)
export BACKUP_WRITE_QUIESCED=YES

/bin/sh scripts/postgres-backup.sh
```

产物：

| 文件 | 用途 |
|---|---|
| `database.dump` | `pg_dump` custom-format 备份 |
| `database.dump.sha256` | dump SHA256 |
| `authority.tsv` | 31 张权威表的行数和确定性全行 SHA256 |
| `authority.tsv.sha256` | 权威快照文件 SHA256 |
| `metadata.env` | 格式、Schema/PostgreSQL 版本、时间和实测 RPO；不含连接信息或 secret |

脚本使用 repeatable-read、read-only 单事务流出权威行，再在本地按表计算 SHA256，不依赖 `pgcrypto`。投影、Outbox、auth 临时态和审计日志不进入权威快照。

## PostgreSQL 恢复演练

先选择从未使用过的新库名。默认失败时清理新建库，成功后保留以供人工检查；演练要求自动清理时设置 `RESTORE_CLEANUP=always`。

```bash
export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"
export PGHOST=127.0.0.1
export PGPORT=55432
export PGUSER=wiki
export PGSSLMODE=disable
export BACKUP_DIR=/secure/backups/anby-pg-20260723T070105Z
export RESTORE_DB_NAME=wiki_restore_m7t06_20260723
export RESTORE_REPORT_DIR=/secure/reports/wiki_restore_m7t06_20260723
export RESTORE_ENVIRONMENT=drill
export RESTORE_CONFIRM="CREATE_NEW_DATABASE:$RESTORE_DB_NAME"
export RESTORE_CLEANUP=always

/bin/sh scripts/postgres-restore-verify.sh
```

固定执行顺序：

1. 校验 dump、权威快照 SHA256 和 dump 目录。
2. 确认源/目标不同、目标不存在，再执行 `createdb` 和 `pg_restore --exit-on-error`。
3. 比较恢复库与备份的权威计数/SHA256。
4. 运行第一次只读 `cmd/doctor`。
5. 以 PostgreSQL Search backend 运行 `cmd/worker -rebuild-all`。
6. 运行第二次 doctor，再次比较权威计数/SHA256。
7. 写出 `restore-metrics.env`，按清理策略删除本次新建库。

若第一次 doctor 仅暴露需要按既有流程重放的 dead Outbox，可显式增加：

```bash
export RESTORE_REPLAY_DEAD=YES
```

脚本会保留第一次 doctor 报告，再对新恢复库运行 `cmd/worker -replay-dead`，随后全量重建。最终 doctor 仍有 error/critical 时整体失败。不得用该选项掩盖 `DOC_*`、`REF_*`、证据链或其他权威问题。

## 对象存储备份

优先使用 AWS CLI；未安装时可设置 `OBJECT_CLI=mc` 使用 MinIO Client。endpoint、bucket 和凭据均从环境读取。

```bash
export S3_ENDPOINT=https://s3.example.internal
export S3_BUCKET=wiki-assets
export S3_REGION=us-east-1
export S3_ACCESS_KEY=...
export S3_SECRET_KEY=...
export OBJECT_CLI=aws
export BACKUP_DIR=/secure/backups/anby-objects-$(date -u +%Y%m%dT%H%M%SZ)

/bin/sh scripts/object-storage-backup.sh
```

产物包含 `objects/`、`manifest.tsv`、`manifest.tsv.sha256` 和 `metadata.env`。manifest 每行只记录 SHA256、字节数和 key，不记录对象正文、endpoint 或凭据。

## 对象存储恢复演练

```bash
export BACKUP_DIR=/secure/backups/anby-objects-20260723T070000Z
export RESTORE_BUCKET=wiki-assets-restore-20260723
export RESTORE_ENVIRONMENT=drill
export OBJECT_RESTORE_CONFIRM="CREATE_NEW_BUCKET:$RESTORE_BUCKET"
export OBJECT_RESTORE_REPORT=/secure/reports/wiki-assets-restore-20260723.env
export OBJECT_RESTORE_CLEANUP=always

/bin/sh scripts/object-storage-restore-verify.sh
```

恢复先验证 manifest 自身和本地镜像，再创建新 bucket、上传全部对象、重新下载到临时目录并重算 manifest。任意缺失、额外对象、size 或 SHA256 差异都会失败。默认失败时删除仅由本次脚本创建的 bucket；绝不删除预先存在的 bucket。

## RPO/RTO 口径

- PostgreSQL RPO：写入静默开始到备份包完成校验的实测时长；持续写入系统应改用 WAL/PITR，不能把本脚本的静默窗口当作持续备份。
- 对象存储 RPO：bucket 镜像开始到 manifest 完成的时长；镜像期间有写入时应依赖 bucket versioning/一致快照能力。
- RTO：新资源创建开始，到恢复、完整校验、Projection/Search 重建和最终 doctor 完成。
- 数据量较小时 `go run` 编译时间会占 RTO；生产测量应使用同版本预构建 doctor/worker 二进制。

## 失败处置

- checksum 或权威快照不一致：隔离备份，不得继续上线；重新备份并排查静默窗口内写入。
- 第一次 doctor 失败：保留报告；权威问题按 Proposal/领域服务处理，dead Outbox 仅按显式重放流程处理。
- 重建失败：保留恢复库和报告，检查 Builder/Search backend；不要直接改 Projection 状态。
- 最终 doctor 失败：恢复验收失败，不得切流。
- 清理失败：按报告中的目标名人工确认其确为演练新资源后再删除；不得使用模糊匹配批量清理。
