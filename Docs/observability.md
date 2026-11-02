# 可观测性运行手册

## 边界

M7-T04 落实 ADR-0009 的 Metrics 与 Trace 基线。日志继续使用 `log/slog`。所有
Prometheus 标签和 Span attributes 都必须来自固定枚举或受控配置，不允许加入
URL 原始路径、页面标题、Prompt、来源内容、Token、Secret、用户信息或任何业务 ID。

API 与 Worker 各自创建独立 Prometheus registry，不使用进程全局 registry：

- API：同一 HTTP 服务的 `GET /metrics`。
- Worker：独立 `WORKER_METRICS_ADDR`，默认 `:9091`，根路径直接输出指标。
- Worker 每 `OBSERVABILITY_DB_INTERVAL`（默认 30 秒，最小 5 秒）执行只读聚合 SQL，
  采集 Outbox、Projection、Importer 与 AI usage，不修改领域写路径。

## 指标

| 指标 | 标签 | 含义 |
|---|---|---|
| `wiki_http_requests_total` | `service,method,route,status_class` | route 使用 chi pattern，状态只保留 `2xx` 等类别 |
| `wiki_http_request_duration_seconds` | `service,method,route` | API 请求延迟 |
| `wiki_http_panics_total` | `service,route` | Recoverer 捕获的 panic |
| `wiki_page_publish_duration_seconds` | `service,result` | 发布成功/失败耗时，`result` 仅为 `success/failure` |
| `wiki_outbox_events` | `service,status` | pending/claimed/retrying/dead/backlog 当前数量 |
| `wiki_outbox_oldest_backlog_age_seconds` | `service` | 最老积压事件年龄 |
| `wiki_projection_states` | `service,state` | error/stale 投影数量 |
| `wiki_importer_jobs` | `service,status` | 导入任务当前状态数量 |
| `wiki_importer_job_duration_seconds_sum/count` | `service,status` | 已结束任务累计耗时与样本数 |
| `wiki_importer_stages` | `service,stage,status` | 固定七阶段状态数量 |
| `wiki_importer_stage_duration_seconds_sum/count` | `service,stage,status` | 已结束阶段累计耗时与样本数 |
| `wiki_ai_requests` | `service,status` | 持久化 AI usage 行数；协议 2 中每次物理尝试一行 |
| `wiki_ai_tokens` | `service,direction,status` | 已报告 input/output token 累计下界 |
| `wiki_ai_latency_seconds_sum` | `service,status` | AI 请求累计延迟 |

Importer 与 AI 指标直接汇总持久化记录，进程重启不会丢失；它们是 gauge 快照，
`*_sum/count` 可用于计算指定状态的历史平均耗时。

## Trace

默认不导出 Trace。启用配置：

```bash
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_TRACE_SAMPLE_RATE=1
```

`OTEL_EXPORTER_OTLP_ENDPOINT` 遵循 OpenTelemetry 标准 URL 格式，必须包含
`http://` 或 `https://` scheme。开发 Collector 通常配合
`OTEL_EXPORTER_OTLP_INSECURE=true` 使用 `http://`。

关键 spans：

- HTTP server：`METHOD /chi/{route}`，支持 W3C `traceparent`/baggage 传播。
- Outbox：`outbox.process`，仅记录受控 `event_type`。
- Importer：`import.process`，仅记录是否领取任务和 `url/upload` 来源类型。
- AI：`ai.generate`，记录配置中的 provider/model、固定结果和尝试次数，不记录 Prompt。

启用时 exporter 初始化失败会阻止进程启动；进程退出时最多等待 5 秒 flush/shutdown。

## 本地运行

开发环境不再提供 Docker 观测 profile。先按 `Deploy.md` 配置外部依赖并启动
API/Worker：

```bash
sh scripts/dev.sh api worker
```

- API metrics：<http://localhost:8080/metrics>
- Worker metrics：<http://localhost:9091/>

需要 Prometheus 或 OTLP Collector 时自行提供，并用上述 metrics 地址及
`OTEL_EXPORTER_OTLP_ENDPOINT` 接入；应用只依赖标准 Prometheus/OTLP 协议，
不绑定具体观测后端。

## 告警与检查

仓库不再内置 Prometheus 告警文件。接入外部观测系统时至少覆盖 target down、
API 5xx/p95、发布 p95、Outbox 老化/死信、Projection error 和 Importer failure。
M7-T05/ADR-0012 的内部 Beta 初始阈值为 API 5xx `<1%`、API p95 `<=1s`、
发布 p95 `<=2s`；容量基线及运行流量出现稳定偏差时通过后续 ADR 校准。

应用内 metrics/tracing 配置随 `make build-go` 编译检查；运行时接入需在目标环境
人工确认指标端点、OTLP 连接与告警规则。

供应商监控还应覆盖 `rate_limited`、`quota_exhausted`、`timeout`、
`:usage_unreported` 和 `ai.usage_record_failed=true`；未知用量或审计失败不能按零成本
放行预算。当前指标没有单独的未知用量标签，需由只读 Usage 查询和 Trace 补充；
不能向 Prometheus 标签写入请求 ID。接收器、实际投递和响应责任仍由外部观测系统配置。

协议 2 下 Usage ID 可与供应商边界的 `X-Client-Request-Id` 核对。金额需要进一步核对
供应商返回的缓存分项、执行时段价格、币种和最终账单；余额变化只能证明共享账号的净扣减。
账号和管理员连接测试已改为经 Gateway 持久审计文字/图片探针，使用独立版本化
`ai-config-extraction-probe-v1`、`ai-config-vision-probe-v1`，每个 API 最多四个在途调用；
该补充修改尚待发布。固定部署上限仍为最多 2 API/2 Worker、
2 个导入任务、8 个导入模型调用，其他模型入口另计；这个限制依赖运维控制副本数。
共享账号或自动扩容前必须落实全局租约、背压及经负责人确认的账号预算。

当前固定拓扑接受受控 HTTP 故障、真实模型回归与持久用量对账作为供应商边界验收；
不主动耗尽或洪泛共享账号。完整逐笔账单、独立人工极小区域抽查和外部告警投递
按用户授权的范围取舍纳入运维回归，不解释为已完成实测。接入告警接收器、扩大共享
账号范围或自动扩容时再验证实际投递与响应责任；未知用量继续保留，不能计为零成本。

## 故障排查

1. `up == 0`：确认 API/Worker 正在宿主机监听，并检查外部 Prometheus 的抓取地址与网络。
2. Outbox age/dead 上升：先看 Worker 日志和 `wiki_projection_states`，再使用现有
   `worker -metrics`、`worker -check-consistency` 或受控 dead-letter replay。
3. Importer failure 上升：按固定 stage/status 定位，再用日志中的 job_id 串联；
   指标与 span 本身不携带 job_id。
4. Trace 缺失：检查 `OTEL_ENABLED`、endpoint/insecure 配置和 Collector 日志。
