# scripts 目录

日常开发和质量检查优先使用仓库根的 `make <target>`。运行 `make help` 可查看分组后的公共入口。

本目录只保存 Makefile 背后的实现脚本，以及必须由操作者显式执行的生产运维脚本：

| 路径 | 职责 |
| --- | --- |
| `dev.sh` | 读取根 `.env`，探测依赖并启动 API、Worker、Web |
| `dev-pg.sh` | 管理免 Docker 的本地 PostgreSQL |
| `dev-probe.go` | `dev.sh` 使用的连接探测辅助程序 |
| `check-*.sh` | 契约、迁移和生产部署静态检查 |
| `perf-db.sh` | 重建独立性能基准数据库 |
| `search-acceptance.py`、`search-zh.json` | 在新建的隔离索引上三轮评估固定中文查询，并清理该索引 |
| `deploy.sh` | 生产部署、迁移、诊断和回滚入口 |
| `deploy-production.sh` | Linux/root 一键拉取、初始化、构建、部署、TLS 与验收；可传 `--no-pull` |
| `cleanup-production.sh` | 成功部署后清理旧业务镜像、退出容器、构建缓存和遗留部署备份 |
| `prepare-production.py` | 创建根 `.env`、随机机密与 data/Secret；已有配置保留，发布标识取 Git SHA |
| `production_environment.py` | dotenv 数据读取、公开设置校验、进程环境注入与宿主机 Nginx 的精确代理 IP 刷新；不执行环境文件内容 |
| `production-common.sh` | 宿主机脚本共用的域名读取与 Compose 调用；不单独执行 |
| `bootstrap-administrator.py` | 在 loopback API 初始化首个管理员并关闭公开注册；保存 Secret 凭据 |
| `install-nginx.sh` | 安装 `bootstrap/production` 站点，失败恢复；`--render` 只输出预览 |
| `enable-tls.sh` | Certbot 签发/复用、安装续期 hook、启用 `certbot.timer` |
| `reload-nginx.sh` | 成功续期后检查并 reload Nginx，不申请证书 |
| `smoke-production.sh` | 验证实际 HTTPS、API readiness、主页、Worker 与 Doctor；不写业务数据 |
| `import-acceptance.py` | 按 JSON 语料清单执行真实导入、Chunk/阶段/质量断言，并拒绝测试 Proposal |
| `backup/` | PostgreSQL 与对象存储的备份/恢复演练 |

生产入口与 `backup/` 下的脚本可能修改外部环境，因此不包装成日常 Make 目标。请按
`Deploy.md` 和 `Docs/runbooks/` 中的步骤设置确认令牌后显式执行。日常一键部署不调用
`backup/`，也不创建 PostgreSQL、对象存储、环境或 Nginx 持久化备份。

搜索基准必须使用 `wiki_perf_*` 数据库和索引。`cmd/perf` 在启用
`SEARCH_SEMANTIC_ENABLED=true` 并显式设置 `MEILI_EMBEDDER_NAME`、
`MEILI_EMBEDDER_SOURCE`、`MEILI_EMBEDDER_MODEL` 后，额外测量 8 并发关键词、
混合、语义及过滤查询；报告仍写入 `/tmp`。`-search-only` 可复用已经生成的性能数据库，
重建搜索索引并测量查询；库名、标记、环境与确认值校验仍然生效。此命令测量批量索引，
不代表 Worker 的逐页全量 Projection 重建时间。中文质量回归使用：

```bash
python3 scripts/search-acceptance.py \
  --base-url http://127.0.0.1:7700 \
  --index wiki_perf_search_quality \
  --report /tmp/anby-search-quality.json
```

需要认证时通过环境传入 `MEILI_API_KEY`。脚本拒绝覆盖已有索引，只清理自己创建的
索引；每轮、每类查询的语义 Recall@5 必须至少 0.90、MRR 至少 0.75。默认清单包含
24 篇合成技术短文和 48 条预设目标查询，尚未经独立人工复核；它是固定回归样本，
不能替代真实生产语料验收。

`import-acceptance.py` 从环境读取
`ANBY_ACCEPTANCE_IDENTIFIER`、`ANBY_ACCEPTANCE_PASSWORD`，凭据不会写入报告。
语料清单中的文件路径相对清单文件解析：

```json
{
  "version": 1,
  "cases": [
    {
      "name": "complex-pdf",
      "file": "complex.pdf",
      "route_mode": "auto",
      "expected_statuses": ["succeeded", "action_required"],
      "expected_stages": {"parse": "succeeded", "plan": "succeeded"},
      "required_chunk_types": ["page", "table"],
      "minimum_pages": 2,
      "minimum_located_chunks": 1,
      "minimum_plan_routes": 2,
      "maximum_plan_routes": 3,
      "expected_route_action_counts": {"create": 2},
      "required_route_titles": ["Architecture", "Operations"]
    }
  ]
}
```

```bash
ANBY_ACCEPTANCE_IDENTIFIER='admin' \
ANBY_ACCEPTANCE_PASSWORD='...' \
python3 scripts/import-acceptance.py \
  --base-url https://wiki.example.com \
  --manifest /tmp/import-corpus/manifest.json \
  --report /tmp/import-acceptance.json
```

成功任务生成的 Proposal 会被治理 API 拒绝，`action_required` 任务会被取消；脚本
不会批准或发布测试内容。失败任务可用 `expected_error_code`、
`require_source_version` 和 `expected_stages` 断言原子性。多页面计划可用
`minimum_plan_routes`、`maximum_plan_routes`、`expected_route_action_counts`、
`required_route_actions`、`required_route_titles` 与 `required_related_titles`
断言路由结构。
