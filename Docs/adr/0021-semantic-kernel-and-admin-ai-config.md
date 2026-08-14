# ADR-0021：Semantic Kernel Sidecar 与管理员 AI 配置

- 状态：已接受
- 日期：2026-08-11

## 背景

来源抽取原先由 Go Worker 直接调用 OpenAI-compatible/DeepSeek HTTP API，供应商、
地址、模型和密钥全部来自 Worker 环境变量。线上出现较多超时与结构化输出 bad case，
而修改模型配置必须编辑部署环境并重建或重启 Worker。

Microsoft Semantic Kernel 官方实现支持 Python、.NET 和 Java，不提供 Go SDK。直接把
整个导入流程迁入 Python 会破坏既有领域服务、Prompt Registry、用量审计与事务边界。

## 决策

1. 新增无状态 Python Semantic Kernel Sidecar。它只负责模型调用、有限重试、JSON
   解析及输出 Schema 预校验，不访问数据库、对象存储或用户会话。
   Sidecar 基于 Semantic Kernel 的 `ChatCompletionClientBase`、`ChatHistory` 与执行设置
   实现精简的 OpenAI-compatible Connector；不安装未使用的 Azure、Realtime/WebRTC
   等可选连接器依赖。
2. Go Worker 继续拥有导入 Job、Prompt Registry、最终 Schema/证据校验、用量记录、
   Proposal 生成和所有权威写入；Sidecar 不能发布 Revision 或写 Knowledge。
3. Go 与 Sidecar 使用版本化内网 JSON 协议和共享令牌。每次请求由 Go 短暂传入解密后的
   Provider 配置；Sidecar 不持久化配置，也不记录 Prompt、来源、响应或密钥。
4. Provider、Base URL、模型、响应格式、模型最大输入 Token、来源 Chunk 字符数、超时、尝试次数和 API Key 改由管理员中心
   `/admin/ai` 管理。配置版本化存入现有 `wiki_site.settings_json.ai_runtime`，不新增
   迁移；API Key 使用 AES-256-GCM 加密，公开接口只返回是否已配置。
5. 环境变量只保留基础设施机密：`AI_CONFIG_MASTER_KEY`、
   `AI_KERNEL_INTERNAL_TOKEN` 和内网 `AI_KERNEL_URL`。前两项仍由仓库外 `0600`
   部署环境文件提供。
6. 配置缺失、禁用或密文不可解时 Worker 不领取新 Job，已排队任务保持 queued；配置
   恢复并启用后继续消费。
7. Worker 把最大输入 Token 作为模型能力先验，为 Prompt/Schema 预留空间后按 Token
   预算分批抽取，并设置独立的输出安全批量上限；来源解析使用管理员配置的 Chunk
   字符上限（默认 32000）。输出仍被截断时只二分对应的临时模型窗口，最终由 Go
   服务完成跨批候选去重、ID 重建、逐字证据核验以及向不可变 SourceChunk 的回映射。

## 影响

- 管理员可以不重新部署应用就切换模型和运行策略，密钥不会通过读取接口返回。
- Semantic Kernel 侧的结构化重试降低常见 bad case，但不能替代 Go 侧最终契约与证据
  校验；供应商质量仍需用固定样本持续评估。
- 生产拓扑新增一个私网、只读、非 root 的 Python 容器及一套独立依赖锁定。
- `AI_CONFIG_MASTER_KEY` 必须备份并保持稳定；丢失或误轮换会使已保存的 Provider
  密钥无法解密，需要管理员重新填写。
- Semantic Kernel 1.x 的后续演进通过内部 v1 协议隔离，升级或迁移到其他 Agent
  Runtime 不得改变 Go 领域边界。

## 取代范围

- 取代 ADR-0017 中“`AI_API_KEY` 写入部署环境并注入 Worker”的部分。
- 不改变 ADR-0017 对数据库、对象存储、搜索及基础设施主密钥的环境文件保护要求。
