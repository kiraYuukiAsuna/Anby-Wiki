# ADR-0009：可观测性基线

状态：已实现（M7-T04）
日期：2026-07-23

## 背景

实施方案要求结构化日志、Trace、Metrics，且 `request_id/job_id/page_id/revision_id/change_batch_id` 可串联。

## 决策

- 日志：Go 标准库 `log/slog`，JSON Handler，默认字段 `level/time/msg/service/request_id`；领域操作补充 `page_id/revision_id/change_batch_id/job_id`。**禁止**记录来源全文、Prompt、密钥、Token、个人敏感信息（实施方案 §4.3）。
- Trace：OpenTelemetry Go SDK，`internal/platform/observability` 提供初始化与 HTTP/DB 中间件；P0 导出到 stdout/OTLP（本地用 Jaeger all-in-one 可选），不强制后端。
- Metrics：Prometheus client_golang，`/metrics` 端点；M0 先暴露 go runtime + HTTP 请求延迟/计数，投影积压、导入耗时等在对应里程碑补齐。
- 请求 ID：Nginx 生成或透传 `X-Request-ID`，API 中间件落入 context 与响应头，前端生成客户端自动附带（ADR-0007 薄封装）。
- 前端：M0 仅保证请求 ID 透传与错误边界日志；不引入前端 APM。

## 备选方案

- zap/zerolog：性能更好，但 slog 零依赖且够用。
- Datadog 等商业 APM：P0 无预算与必要性。

## 影响

- M0-T06 的健康接口即带 request_id 串联示例。
- M7-T04 已补全独立 registry、API/Worker metrics、OTLP 初始化、数据库定时采集和告警规则。
- Prometheus 标签与 Span attributes 只允许固定枚举或受控配置，不携带原始路径、内容、Prompt、凭据、个人信息或业务 ID。
- Worker 的持久化指标采用低频只读聚合 SQL，而非侵入每条领域写路径；默认 30 秒刷新。
- 本地 OTel Collector 默认使用 debug exporter，不绑定 Jaeger 或商业 Trace 后端。
- 指标目录、配置与故障排查见 `Docs/observability.md`。
