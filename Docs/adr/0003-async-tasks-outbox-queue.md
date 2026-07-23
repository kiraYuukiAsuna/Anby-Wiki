# ADR-0003：异步任务、Outbox 与队列

状态：已接受（M0-T02）
日期：2026-07-21

## 背景

发布事务需同步落 `outbox_event`，Worker 异步消费（投影、渲染、搜索）。需要决定事件投递与任务队列机制，满足幂等、不丢事件、可重放。

## 决策

- **Outbox 模式 + PostgreSQL 轮询**：Worker 使用 `SELECT ... FOR UPDATE SKIP LOCKED` 领取 `outbox_event`，处理成功后同事务标记完成；失败指数退避，超过上限进入死信（`status='dead'`）。
- 每个事件携带 `aggregate_type/aggregate_id` 与来源版本（如 `revision_id`），消费者必须幂等并做版本防护（旧 Revision 任务不得覆盖新投影）。
- **P0 不引入独立队列库**（asynq、River 等）：Postgres 已能提供领取/续租/重试语义，减少一个运维组件。Redis 只用于缓存、限流和短期协调，不承载不可丢失的任务。
- 若 P1/P2 出现 Postgres 轮询瓶颈（容量基线证明），再评估 River（同事务优势）或 asynq，属局部替换。

## 备选方案

- Redis Stream / asynq：引入第二套持久化语义，任务与业务事务无法原子提交，排除于权威链路。
- Kafka/NATS：P0 规模（十万级页面）无理由引入，违反 §11 非目标精神。

## 影响

- M3-T01 的 Outbox 消费框架按上述语义实现：领取、续租、完成、失败、退避、死信、幂等键。
- Worker 崩溃恢复测试（MT-M3-OUTBOX-RELIABILITY）是发布阻断项。
