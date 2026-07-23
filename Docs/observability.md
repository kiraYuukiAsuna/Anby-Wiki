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
| `wiki_importer_stages` | `service,stage,status` | 固定六阶段状态数量 |
| `wiki_importer_stage_duration_seconds_sum/count` | `service,stage,status` | 已结束阶段累计耗时与样本数 |
| `wiki_ai_requests` | `service,status` | 持久化 AI usage 请求数 |
| `wiki_ai_tokens` | `service,direction,status` | input/output token 累计量 |
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

先启动应用依赖和 API/Worker，再启动观测 profile：

```bash
make infra-up
make dev-api
make dev-worker
make infra-observability-up
```

- Prometheus：<http://localhost:9090>
- API metrics：<http://localhost:8080/metrics>
- Worker metrics：<http://localhost:9091/>
- OTel gRPC/HTTP：`localhost:4317` / `localhost:4318`

Collector 默认把 trace 摘要写到自身日志，不包含 Jaeger。需要持久化/查询时，应在部署
Task 中替换 exporter；应用只依赖标准 OTLP，不绑定具体后端。

## 告警与校验

`infra/local/observability/alerts.yml` 覆盖 target down、API 5xx/p95、发布 p95、
Outbox 老化/死信、Projection error 和 Importer failure。M7-T05/ADR-0012 将内部
Beta 初始阈值定为 API 5xx `<1%`、API p95 `<=1s`、发布 p95 `<=2s`；容量基线及运行
流量出现稳定偏差时通过后续 ADR 校准，不为消除告警直接放宽。

```bash
make observability-config-check
```

该命令始终执行 Go/YAML 静态校验；Docker daemon 可用时追加 Compose、`promtool` 和
OTel Collector 官方校验。Docker 不可用不会跳过静态校验。

## 故障排查

1. `up == 0`：确认 API/Worker 正在宿主机监听，并检查 Docker 的
   `host.docker.internal` 映射。
2. Outbox age/dead 上升：先看 Worker 日志和 `wiki_projection_states`，再使用现有
   `worker -metrics`、`worker -check-consistency` 或受控 dead-letter replay。
3. Importer failure 上升：按固定 stage/status 定位，再用日志中的 job_id 串联；
   指标与 span 本身不携带 job_id。
4. Trace 缺失：检查 `OTEL_ENABLED`、endpoint/insecure 配置和 Collector 日志。
