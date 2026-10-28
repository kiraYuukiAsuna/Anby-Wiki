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

Worker 首次启用来源导入时，幂等登记当前 `source-extraction-v8` Prompt；该版本明确区分
临时候选 UUID 与持久化 Entity ID，并提供运行时支持的 Entity type / Claim property
词表与关系方向约束；作品/RFC 属性有独立语义，禁止用 `part_of` 代替发布组织，也禁止
任何 Entity 关系指回主体自身；`instance_of` 只允许有明确分类措辞的单值候选，禁止退化为
通用关联，并识别 text/table/chart/figure/page Chunk。页面规划使用
`source-import-plan-v10`，多窗口结果由服务端确定性合并，并由
`source-import-plan-fidelity-v6` 对照原始 Chunk 做证据化遗漏审计和章节级修复。
规划契约允许用 `code` 逐字表达固定宽度图和协议布局；单路由计划还会由服务端确定性
补齐模型遗漏，并把布局逐字覆盖率纳入最终 fidelity。
PDF 与 PNG/JPEG 的 `document-page-layout-v1` 和 `document-region-analysis-v1`
通过同一 Gateway 发送布局图和区域裁图，继续使用 Prompt Registry、结构化 Schema
双重校验、重试和用量审计。两阶段实际 Prompt 版本均参与解析身份；模型不负责生成
页码、区域位置、阅读序号和持久 ID。旧 `document-page-analysis-v1` Schema 保留用于
模型连接探针，Worker 不再调用整页转写 Prompt。
两者都把用户指定范围和块数上限视为跨窗口全局约束，并要求每条正文陈述由引文直接支撑；
导航标签、标题、孤立代码或示例值不能支撑宽泛说明。
模型规划使用只包含语义内容和 `chunk_id + quotation` 的轻量 Schema；服务端再生成完整
ImportPlan v1。保真审计使用独立的只允许既有 route_index 和精确 evidence 的内部 Schema；最终质量分由
服务端计算而非采信模型自评分。保真 v6 明确禁止翻译 evidence 引文，并在三次语义纠正耗尽后
隔离无法核验的可选修复、同步撤销对应覆盖率增益。Extraction 输出 Schema 继续直接复用
`importer` 内嵌的权威副本。Prompt key
升级使用新 key，避免覆盖或静默改写运维已激活的旧 Prompt。

Semantic Kernel 边界在 `json_object` 模式下会把权威 JSON Schema 一并放入系统消息，
并在纠错重试中提供具体的校验路径；DeepSeek 请求显式设置输出 token 上限并检查
`finish_reason=length`，避免截断内容被误判成普通供应商失败。个人中心的文档理解模型
连接测试使用完整的 Extraction v1 Schema，而不是仅验证一个最小 JSON 对象。
