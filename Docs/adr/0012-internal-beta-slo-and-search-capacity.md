# ADR-0012：内部 Beta SLO 与搜索容量门槛

状态：已接受（M7-T05）
日期：2026-07-23

## 背景

M7 需要在十万级页面、Revision 和投影下给出可量化 SLO。ADR-0006 选择 PostgreSQL
FTS 作为 P0 搜索 Adapter，但把是否保留的容量判断交给 M7-T05。

## 决策

- 内部 Beta 的 API 5xx 比例目标为 `< 1%`。
- 阅读、搜索、反链目标为 p95 `<= 1s`、p99 `<= 2s`。
- 发布、审核、导入关键数据库路径目标为 p95 `<= 2s`、p99 `<= 5s`。
- Outbox 最老积压目标为 `< 5min`；死信仍为零容忍。
- 以独立 `wiki_perf_*` 数据库的 100k full 报告作为搜索去留依据；smoke 只验证工具。
- PostgreSQL FTS 仅在 full 搜索满足延迟目标、错误率 `< 1%`、无临时文件溢出，
  且执行计划合理使用 GIN 时保留。否则不改业务接口，另开 Task 接入 Meilisearch。
- 基准必须记录机器信息、吞吐、错误率、数据库大小和 `EXPLAIN ANALYZE BUFFERS`；
  一次性 JSON 输出写入 `/tmp` 或外部测试制品存储，不纳入仓库。

## 安全与测量边界

- 性能命令不读取 `DATABASE_URL`，要求独立变量、库名前缀、数据库 comment、
  显式确认值和非生产环境五项校验，并拒绝非空数据库。
- 权威 Page/Revision 测试数据经领域服务写入；关系/搜索投影可批量生成。
- 本机结果是热缓存、单进程基线。发布前仍需 staging 并发和故障场景验证。

## 影响

- M7-T04 的 API 错误率阈值从 5% 收紧到 1%，新增 API/发布 p95 告警。
- 100k full 实测搜索 p95 `918.508ms`、p99 `926.740ms`、错误率 `0%`，但吞吐仅
  `1.146 req/s`；真实 Adapter count 慢查询顺序扫描 100,000 行并耗时 `227.595ms`，
  完整 count+hits 路径无并发余量。因此 **PostgreSQL FTS 不保留为内部 Beta 最终
  搜索实现**。M7-T11 已在 `SearchAdapter` 后接 Meilisearch，production 配置强制
  使用该后端，并保留 Postgres 作为可重建 staging/fallback。
- Meilisearch 已在 4 vCPU Linux 云端用同一工具完成 100k full：索引 100,000
  文档耗时 `36.013s`，常见词查询 p95 `5.936ms`、p99 `6.638ms`、吞吐
  `219.101 req/s`、错误率 `0%`，搜索容量门禁闭环。
- 其余 100k 路径 p95 均低于 `1ms`；数据库大小 `573,118,131` bytes，无临时块溢出。
- M7-T03 的 `sharp` production high 漏洞仍是发布 blocker，本 ADR 不豁免安全门禁。
