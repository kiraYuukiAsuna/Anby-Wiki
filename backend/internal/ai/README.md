# AI Gateway（M6-T01）

`Gateway` 是业务层唯一模型边界：业务请求只包含 provider/model、Prompt key、模板变量和
可选 ImportJob/Run ID，不依赖任何供应商 DTO。Gateway 从 PostgreSQL Prompt Registry
读取当前版本，渲染模板，执行并发限制、单次超时、临时错误指数退避、JSON Schema 校验，
并为成功、超时、供应商失败和非法结构记录模型/Prompt 版本、token 与耗时。

内置 `OpenAICompatibleProvider` 调用 `/chat/completions` 的严格 `json_schema` 输出模式；
`DeepSeekProvider` 使用 DeepSeek 的 `json_object` 输出模式并关闭 thinking，返回内容仍由
Gateway 使用同一份权威 JSON Schema 严格校验；JSON Object 抽取固定
`temperature=0`，避免同一来源在重试间随机变为空候选。两者都不会把 Authorization、
请求 Prompt 或供应商错误响应体写入错误；只记录 `http_400` 等稳定安全错误码。429、
408 和 5xx 标记为可重试，其他 4xx 不重试。新增供应商时实现窄 `Provider` 接口，
供应商 DTO 必须留在本包。

Worker 首次启用来源导入时，幂等登记当前 `source-extraction-v6` Prompt；该版本明确区分
临时候选 UUID 与持久化 Entity ID，并提供运行时支持的 Entity type / Claim property
词表与关系方向约束；作品/RFC 属性有独立语义，禁止用 `part_of` 代替发布组织，也禁止
任何 Entity 关系指回主体自身。页面规划使用 `source-import-plan-v5`，多窗口结果另经
`source-import-plan-consolidate-v3` 收敛，并由 `source-import-plan-fidelity-v3` 对照原始
Chunk 做证据化遗漏审计和章节级修复。规划与收敛输出复用 ImportPlan v1 权威 Schema，
保真审计使用独立的只允许既有 route_index 和精确 evidence 的内部 Schema；最终质量分由
服务端计算而非采信模型自评分。保真 v3 明确禁止翻译 evidence 引文，并在三次语义纠正耗尽后
隔离无法核验的可选修复、同步撤销对应覆盖率增益。Extraction 输出 Schema 继续直接复用
`importer` 内嵌的权威副本。Prompt key
升级使用新 key，避免覆盖或静默改写运维已激活的旧 Prompt。

Semantic Kernel 边界在 `json_object` 模式下会把权威 JSON Schema 一并放入系统消息，
并在纠错重试中提供具体的校验路径；DeepSeek 请求显式设置输出 token 上限并检查
`finish_reason=length`，避免截断内容被误判成普通供应商失败。管理员连接测试使用完整的
Extraction v1 Schema，而不是仅验证一个最小 JSON 对象。
