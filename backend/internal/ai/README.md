# AI Gateway（M6-T01）

`Gateway` 是业务层唯一模型边界：业务请求只包含 provider/model、Prompt key、模板变量和
可选 ImportJob/Run ID，不依赖任何供应商 DTO。Gateway 从 PostgreSQL Prompt Registry
读取当前版本，渲染模板，执行并发限制、单次超时、临时错误指数退避、JSON Schema 校验，
并为成功、超时、供应商失败和非法结构记录模型/Prompt 版本、token 与耗时。

内置 `OpenAICompatibleProvider` 调用 `/chat/completions` 的严格 `json_schema` 输出模式。
它不会把 Authorization、请求 Prompt 或供应商错误响应体写入错误；429、408 和 5xx 标记
为可重试，其他 4xx 不重试。新增供应商时实现窄 `Provider` 接口，供应商 DTO 必须留在本包。

Worker 首次启用来源导入时，幂等登记 `source-extraction-v1` Prompt；若运维已激活更高版本，
装配会保留当前活动版本。输出 Schema 直接复用 `importer` 内嵌的权威 Extraction v1 副本。

