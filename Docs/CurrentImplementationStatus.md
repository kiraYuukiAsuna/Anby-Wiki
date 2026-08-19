# 当前实现状态

> 更新时间：2026-08-19
> 产品与能力审计依据：[整体设计方案](WikiDesignOnePage.md)

## 总体结论

`WikiDesignOnePage.md` 定义的模块化单体 API、独立 Worker、权威数据、治理写入、
可重建投影、知识与证据、导入、搜索及平台化主链路已落到代码与 HTTP 契约。适合
用户操作或查看的主要后端能力已在 Web 中提供持久、可重新发现的入口；导入队列、
治理任务、审计活动和后台工具不再只存在于一次性流程页面。协作领域已经具备在线
Yjs update、持久恢复、普通发布与 AI 合并的 sequence CAS、同标签页断线 update
重发、每 100 个已确认 sequence 自动 snapshot/compact 及 Block 级 Presence；
跨标签页离线恢复与多 API 实例实时广播尚未闭环，不能概括为全部协作设计能力均已完成。

当前代码已通过编译、契约、依赖安全和部署静态门禁，并在本机隔离环境完成
PostgreSQL、Redis、MinIO、Meilisearch、API、Linux Worker 与 Next.js Web 的真实
联调。提交 `0196ec6` 已在远端生产拓扑完成六个 OCI target 构建、迁移 gate、Doctor、
滚动替换和健康检查。另一个完全隔离的远端数据库/bucket/index/Redis DB 已执行
149/149 OpenAPI operation handler 探针、核心成功工作流、治理、批量审核、导入、
AI 配置和双用户协作 E2E。“实现完成”仍不等于“生产发布就绪”：目标规模容量、正式域名
安全边界、账号恢复、备份恢复与人工可访问性等仍须验收。详见
[待解决问题](OutstandingIssues.md)。

## 设计能力覆盖与已知边界

| 领域 | 当前实现 |
| --- | --- |
| 页面核心 | Wiki/Namespace、Page、不可变 Revision/ContentSnapshot、稳定 Block ID、历史、Diff、回滚、改名、PageRedirect、BlockRedirect、页面保护 |
| 知识图谱 | Entity/Property/Claim、标签/别名/主标签、Page 主 Entity 绑定、Claim 验证、Entity 合并与回滚、联邦 Wiki/Entity 映射、图谱查询 |
| 证据与媒体 | Source/Version/Chunk/Citation、Asset/AssetRevision、引用校验、相同证据 Citation 幂等复用、按页面聚合并可展开到 Block/Node 的反向使用、按全文首次出现统一编号的 References 投影、逐处正文回链、可审计来源与媒体目录 |
| 结构化内容 | Dataset/View/Record、Component/Version、内置 Entity/Claim 信息框（按页面语言→`und`→任意主标签降级、同属性多值聚合、Entity 可跳转、验证摘要与类型感知排序）、静态与动态 Collection、成员维护、页面反向归属查询与投影；Entity/Claim/Citation 反向使用、页面反链和 Component 依赖在 Web 中按来源页面聚合，并可展开到具体 Block/Node；列表返回真实总位置/页面/区块数并支持游标续页 |
| 治理 | ProposalOperation v1 全部 24 种 Operation（含可原子应用和补偿回滚的 Page 主 Entity 绑定）、预分配 Page/Entity ID 的同批依赖、Operation 集合事务冻结、Wiki 级多目标 Proposal、跨页面 Revision/Block、Claim 及 Page/Entity 身份冲突检测、并发唯一索引冲突到 HTTP 409 的领域映射、预览、风险、ReviewTask、面向 applier/admin 的跨 Actor 待原子应用队列、带分类失败原因和终态跳过语义的批量审核、ChangeBatch、整批补偿回滚、审计事件、ChangeTag、AI Trust 档案/策略、事实一致性；AI Trust 仅作用于预置的 `ai/import` Actor，当前用户导入仍以 human Actor 创建 Proposal |
| 导入与 AI | URL/HTML/文本/PDF/PNG/JPEG/JSON/CSV 获取，图片及扫描 PDF 的中英 OCR、来源标题/作者/发布者/日期/DOI 等安全元数据推导、结构化数据规范化、七阶段进度、幂等 Job、解析成功后原子写入且通过 ImportJob API 可见的不可变 SourceVersion 恢复点与无重复来源的失败重试、Worker 中断任务自动恢复、管理员可配置模型最大输入 Token 与来源 Chunk 字符数（默认 128000 Token / 32000 字符）、按输入预算与输出安全上限并行分批抽取/规划、单个持久 Chunk 内的临时语义窗口二分及证据回映射、截断或结构不合法窗口重试及跨批去重、轻量模型规划 Schema 与服务端机械字段补全、确定性 ImportPlan 合并和结构清理、对照原始 Chunk 的生成后保真审计与证据化章节修复、保真分窗的三次语义纠正、并发真因保留及坏修复隔离与覆盖率回退、服务端五维质量评分及默认 0.70 硬门槛、Semantic Kernel 默认三次结构化调用与纠正重试、精确引文的跨 Chunk 安全纠偏、纯空白差异原文回填及坏候选/坏 Claim 隔离、Entity 缩写/别名归并、Claim 方向/类型/非自引用门禁、`instance_of` 明确分类证据门禁与批内单值属性确定性消歧、作品/RFC 的发布组织、文档编号/类别/状态、更新与废止专用属性、仅由页面路由授权图谱写入、来源概况与智能多页面 create/update/link/ignore 路由、强制单页模式与用户导入要求、证据约束 Typed Block 生成/补丁及正文 Entity 引用、确定性 H2 层级与标准 See also、主 Entity 绑定及信息框、显式且有证据的页面关联与反链投影、规划结果可视化、单 ImportJob 页面+Entity+Claim 复合 Proposal 与同一 ChangeBatch 原子应用、可持久查询的导入队列 |
| 投影与搜索 | Outbox 租约/重试/死信、链接/目录/锚点/章节/渲染/知识使用/References/相关推荐/组件依赖/图谱投影、可解释的链接+Collection+Entity 相关度、PostgreSQL fallback、Meilisearch 关键词/混合/语义检索 |
| 规模与归档 | 章节懒加载、服务端可信 HTML 渲染、Revision 热冷分层与 S3 回源、Projection/Search 重建、容量基准命令 |
| 协作 | Yjs WorkingDocument、持久增量 update、重连游标、普通发布与 AI 合并的 sequence CAS、未确认 update 幂等重发、自动 snapshot/compact、Block 级 Presence、三方合并与人工冲突决议；跨标签页离线恢复和多 API 实例广播仍待实现 |
| 平台 | 本地账号/Session/RBAC、一次性授权码与可撤销 Agent CLI Bearer Token、149 个 OpenAPI operation 和协作 WebSocket 的 JSON CLI、Redis 限流、安全头、OTel/Prometheus、备份恢复、Doctor、多 Wiki 读取隔离、生产部署清单 |

## 关键不变量

- Page、Revision、Entity、Claim、Source、Citation、Proposal 等权威状态只经领域
  Service 写入；Handler、Worker 和投影 Builder 不绕过服务拼接权威写入。
- Revision 与 ContentSnapshot 发布后不可变；链接、渲染、搜索、图谱和反向使用
  投影可删除并由 Outbox 重建。
- Proposal 的客户端合并只提供交互预览；服务端会按权威 AST 重新计算并拒绝不一致
  结果。24 种 Operation 的 Apply/rollback 都有领域语义。
- Page、History、Knowledge 和 Projection 的 UUID 读取路径校验当前 Wiki，避免跨
  Wiki ID 枚举泄漏；Source/Citation/Component 等共享目录的 Page 使用结果也按
  当前 Wiki 过滤。
- Claim 值按 Property 的完整 JSON Schema 校验；Entity label/alias 的语言、长度和
  alias type 同时有服务层与数据库约束；当前内置 Entity 属性禁止 subject=target，
  导入过滤、领域服务和数据库 CHECK 三层兜底。

## 2026-08-17 协作链路复核

- 在线 Yjs update 会按客户端幂等键和服务端单调 sequence 持久化，重连可按游标补发；
  AI Proposal 合并使用 `expected_sequence` CAS，服务端重新计算权威合并 AST。
- 普通 WorkingDocument 发布现要求 `working_document_id + expected_sequence`，
  在 Page/WorkingDocument 行锁事务内同时检查 Revision 基线与 latest sequence；
  前端只从同一 Y.Doc 物化 AST，并在本地 update 获得服务端回显前阻止发布。
- 本地 Yjs update 保留稳定幂等 ID，服务端回显前持续排队；同一标签页短暂断线时，
  客户端先恢复远端 update，再合并并重发未确认 update。浏览器关闭或重载后的离线
  恢复仍依赖 localStorage AST 草稿，不等价于持久 Yjs 操作日志。
- Presence 已接入 BlockEditor 选区、10 秒心跳、服务端排除发送者广播及 30 秒过期，
  编辑页显示远端 Actor 和稳定 Block ID；尚未提供字符级光标装饰和 Actor 名称解析。
- 客户端在 100 个已确认 durable sequence 且无 pending update 时上传最多 16 MiB 的
  Yjs state；服务端经 `SaveSnapshot(..., compact=true)` 同事务保存并删除 covered
  update，再广播 `snapshot_saved`。远端 E2E 已证明旧游标只恢复 snapshot、不再恢复
  被压缩的 update。
- 进程内 Hub 意味着多 API 副本部署前仍需补跨实例广播或连接粘性。
- 因此当前可确认在线增量同步、同标签页断线合并、Block Presence、普通发布/AI CAS、
  Revision 换基和自动压缩；不能把跨标签页离线 CRDT 恢复、字符级协作光标或多副本
  实时广播视为已验收能力。
- 当前 Internal Beta 明确采用单 API 实例，并把浏览器重启后的恢复定义为 AST 草稿
  加人工冲突处理；跨实例广播、持久离线 update 和字符级光标是非阻断后续能力。
- Go 全量测试、Vet/Build、Web TypeScript/ESLint/生产构建、OpenAPI 校验和契约检查
  已通过。BlockEditor 改为 client-only 动态加载后，浏览器验证 `/dev/editor` 从
  SSR `window is not defined` 500 恢复为 200，插入标题后编辑内容与 AST 同步更新。
- 远端 `c84233c` 双用户 E2E 通过正式注册/Session、WebSocket 和 Page API，验证
  Presence 到达对端、update 双端回显、重复 update 保持同一 sequence、陈旧 sequence
  发布返回 409、最新 sequence 发布成功、snapshot compact covered update，以及旧游标
  重连只恢复 snapshot。

## 2026-08-17 远端生产拓扑联调

- `/home/Dev/Anby-Wiki` 已快进到 `c84233c`，生产部署机顺序构建
  `ai-kernel/api/worker/web/migrate` 五个本地镜像；Web 镜像内 Next.js production
  build 和 Linux Go binaries 构建通过。
- 初始化迁移保持版本 1，Migration gate 报告 `current=1 expected=1 compatible=1..1
  dirty=false`；显式清理 6 条过期 Session 后，只读 Doctor 为 0
  info/warning/error/critical。
- PostgreSQL、Redis、MinIO、Meilisearch、Semantic Kernel、API、Worker 和 Web
  均完成滚动替换并保持 healthy；Web `/healthz`、`/readyz` 与首页探针通过。
- 常驻 Worker Prometheus 显示 Outbox backlog/pending/claimed/retrying/dead 均为 0，
  Projection error/stale 均为 0；部署后十分钟内 API/Worker/Web/AI Kernel 无 error、
  panic 或 fatal 日志。
- 部署环境文件原先缺少 AI 基础设施密钥且数据库密码与持久卷不一致；本次从仍健康的
  旧容器恢复相同值到权限 `0600` 的部署环境文件，全程未输出密钥。该环境文件已备份。

## 2026-08-18 正式域名路由恢复

- `anbywiki.kirayuukiasuna.cloud` 的宿主机 Nginx upstream 曾误指向
  `127.0.0.1:3000`，该端口属于 `homepage-web-1`；Anby Wiki 容器始终 healthy，
  正确端口为 4444。
- 站点配置已备份并改为 `proxy_pass http://127.0.0.1:4444`，`nginx -t` 通过后
  平滑 reload。Web 端口进一步收紧为 `127.0.0.1:4444`，不再直接绑定公网。
- 部署环境现设置 `SESSION_COOKIE_SECURE=true` 和正式
  `COLLABORATION_ORIGIN_PATTERNS`；API/Web 重建后保持 healthy。
- 正式 HTTPS 域名的首页、`/healthz`、`/readyz` 均为 200，未登录 Session 为 401；
  HTTPS/WSS 双用户 E2E 再次覆盖 Presence、更新、发布 CAS 和 snapshot/compact。

## 2026-08-18 全 API 修复生产部署

- 远端仓库快进到 `2e14c32`，五个业务镜像顺序构建成功；Migration gate 为
  `current=1 expected=1 compatible=1..1 dirty=false`，部署前后 Doctor 均为 0 issues。
- AI Kernel、API、Worker 已按顺序切换并健康。首次 Web 切换发现部署环境文件没有
  持久化 `WEB_BIND/WEB_PORT`，Compose 回退到已被其他站点占用的 `0.0.0.0:3000`，
  因端口冲突停止；没有回滚或修改权威数据。
- 环境文件备份后补回 `WEB_BIND=127.0.0.1`、`WEB_PORT=4444`，Web 单独重建并健康。
  正式域名 `/healthz` 返回版本 `2e14c32`，`/readyz`、首页和标题探针通过。
- API/Worker/Web/AI Kernel 均使用 `2e14c32` 镜像；Outbox backlog/dead、
  Projection error/stale 均为 0，启动后没有 error、panic 或 fatal 日志。

## 2026-08-18 Agent CLI 生产部署

- 部署前短暂停止 Web/API/Worker 写入，生成 PostgreSQL custom-format 备份、31 张
  权威表的确定性快照及 SHA-256 清单；备份耗时 5 秒，包内校验全部通过。完整恢复
  演练和对象存储恢复验收仍按 `OutstandingIssues.md` 保留。
- 生产数据库已处于 migration version 1，初始化迁移不会重放。本次在备份后以单事务
  additive DDL 补充 `cli_authorization_code`、`cli_access_token` 及其索引/约束；
  migration 保持 `current=1 dirty=false`，变更前后 31 张权威表逐行哈希完全一致。
- 远端源码和 `ai-kernel/api/worker/web/cli/migrate` 六个镜像均为 `f56876a`。
  Migration gate 通过，发布前后 Doctor 均为 0 issue，四个常驻业务容器全部 healthy。
- 正式 HTTPS 域名首页和 `/settings/cli` 返回 200，页面包含 Agent CLI 管理界面；
  匿名和无效 Bearer 均返回带 request ID 的结构化 401。生产 CLI 镜像报告版本
  `f56876a`，列出 149 个 operation，通用 `getHealthz` 调用及缺失 body 的请求校验通过。
- 发布后 Worker 指标可读，API/Worker/Web/AI Kernel 没有 error、panic 或 fatal
  日志。正式域名的登录态授权码创建、兑换和撤销仍需管理员人工登录完成一次冒烟；
  同一闭环已在隔离生产等价拓扑完成自动化 E2E。

## 2026-08-19 Agent CLI 全量接口验证

- 新增 `TestAllOperationsReachHTTPTransport`：从嵌入 OpenAPI 自动生成满足必填约束的
  path/query/header、JSON body 与 multipart fixture，149/149 个 operation 均通过
  `App.Execute("operation.call")` 到达 HTTP transport；Bearer、方法、URL、文件字段和
  响应 metadata 同时校验。
- 新增隔离全栈 `TestAllOperationsAgainstAPI`。首个管理员签发的 CLI Token 对 149 个
  operation 逐个真实调用，状态分布为 31×200、7×201、2×204、3×400、2×401、
  4×403、94×404、2×409、4×422；所有 JSON 都通过对应响应 Schema，0 个 5xx。
- CLI action 成功路径覆盖 `version`、operation 清单/过滤/描述/调用、
  `auth.exchange/status/logout`、`config.show` 和 `collaboration.run`。真实协作覆盖
  WorkingDocument recovery、opaque update、Presence、snapshot/compact；真实文件链路
  覆盖 Asset multipart、二进制 base64 逐字节还原和 Import upload multipart。
- 测试发现并修复两处协议问题：本地输入/契约错误曾被误分为 exit 1
  `request_failed`，现统一为 exit 2 `validation_failed/operation_not_found`，且非法
  logout 参数不会删除本地 Token；`nullable + enum` 的 OpenAPI 兼容转换曾遗漏
  `null` 枚举值，现可正确校验 Proposal 的空 `change_batch_status`。
- 隔离资源清理时曾因加载生产 `.env` 覆盖同名 `MEILI_INDEX`，误删可重建的生产搜索
  index；PostgreSQL 权威数据与对象存储未受影响。已立即全量重建 72/72 页面，
  Meilisearch 恢复 72 文档/72 embedding，Projection consistency 为 792/792、
  0 issue，正式搜索 API 为 200，Doctor 为 0 error/critical。两套隔离 database、
  bucket、index、Redis DB、container 和 SSH tunnel 均已清理。
- 修复提交 `0196ec6` 已完成六镜像构建和滚动发布；Migration gate 为
  `current=1 expected=1 compatible=1..1 dirty=false`，发布 Doctor 为 0
  error/critical。API/Worker/Web/AI Kernel 全部 healthy，生产 CLI 报告
  `version=0196ec6`、149 operation，缺少必填 body 返回 exit 2；发布后四个服务无
  error/panic/fatal 日志，正式健康与搜索调用均为 200。

## Web 信息架构

Web 使用桌面优先的现代百科 Shell：可浏览最近发布条目、专题与实体的百科首页、全部页面目录、固定全局导航、视口内滚动的命令面板、搜索、上下文侧栏、
目录与站点页脚。正文保留维基式可扫描排版，同时使用 shadcn/Radix、Lucide、
Sonner、Geist 和响应式卡片提升工具型页面的可读性。

阅读页由文档目录投影生成带层级编号的 `Contents`，并把 References、Related
topics 与 Related outlines 纳入同一目录；正文后按标准顺序展示可逐处回链的
References、可解释的 Related topics 与由页面所属 Collections 映射的 Related
outlines。References/Related 是 Current Revision
的可重建投影，Collection 归属和 Page 主 Entity 绑定仍来自权威领域数据。

持久入口包括：

- `/wiki/[...title]`、`/pages/[id]`、编辑、历史、Revision 详情、Diff 与页面工具；
  catch-all 路由原生支持标题中的 `/`，404 创建入口会完整保留标题；
- `/entities`、Entity/Claim/Citation 详情、知识图谱与联邦管理；
- `/sources`、`/assets`、Asset Revision、`/datasets`、`/components`、
  `/collections`；
- `/imports` 及每个 Job 的阶段、运行记录和进度详情；
- `/governance` 下的 Proposal 创建/详情、审核队列、待原子应用队列、批量审核、审计活动、页面保护、
  AI Trust、事实一致性与 Revision 存储；
- `/admin/ai` 的加密 Provider 密钥、模型、输出模式、超时、重试和连通性测试；
- `/explore` 的搜索工作台、反向链接和 `/explore/graph` 图谱工作台。

服务端数据统一通过 `apps/web/lib/api.ts` 创建的生成客户端访问，SWR 是唯一服务端
数据客户端缓存；Zustand 只保存本地交互状态。

## 2026-08-16 导入提案身份冲突修复

- Apply 在打开权威写事务前，按 Page 的 `wiki + namespace + normalized_title`
  及 Entity 的 `wiki + canonical_key` 检测审核等待期间产生的身份占用；冲突与
  Proposal 的 `conflicted` 状态在同一事务记录，不再执行注定回滚的写入。
- 唯一索引仍保留最终并发兜底；即使另一个 ChangeBatch 在预检后抢先提交，页面
  标题、Entity canonical key 和重复绑定错误也会映射为 HTTP 409，而不是 500。
- 身份冲突不会自动合并或改写已冻结 Operation。批量审核把它记录为
  `identity_conflict` 并终态跳过；页面/Revision、状态、校验、权限及未知错误分别
  保留独立分类，避免无意义重试。
- Web 会解析契约 Error 响应并展示真实原因；身份冲突详情明确要求基于 Current
  重新导入，同时禁用对该类语义冲突无效的 Current/Proposed 决议按钮。

## 搜索、渲染与存储

- `SearchAdapter` 保持产品无关接口。开发可用 PostgreSQL FTS；生产 Compose 默认
  `SEARCH_BACKEND=meilisearch`，内置固定版本 Meilisearch 服务。
- Worker 在 PostgreSQL 投影事务完成后提交 Meilisearch 文档并等待 task 终态；
  Page 行锁与 Current Revision 复核阻止旧 Revision 覆盖新索引。
- Meilisearch 支持关键词、facet/filter、高亮、混合与语义查询；本地
  HuggingFace embedder 默认不向第三方发送正文。
- 后端从已校验 AST 生成转义后的可信 HTML 和章节投影；阅读页可按章节懒加载，
  目录通过 MutationObserver 感知新章节并跟踪当前滚动位置。分章节渲染仍使用
  全文首次出现的 Citation 顺序，避免懒加载导致同一引用编号漂移。
- Revision snapshot 可由 Worker 归档到 S3，读取 API 对热/冷存储透明回源；
  归档状态、容量统计和手动归档在治理页面可见。

## 安全与依赖

- Web 使用 Next.js `16.2.12`；实际运行树固定到 PostCSS `8.5.25` 和 Sharp
  `0.35.3`。shadcn CLI 位于 devDependencies。
- 新披露的 `brace-expansion` 漏洞通过 `5.0.9` 修复；安装期幂等适配器只让
  遗留 `minimatch@3` 读取新版函数导出，未知版本会直接失败，避免“审计通过但
  ESLint 损坏”。
- 2026-08 新披露的 Nano ID 零长度生成器拒绝服务通过 `nanoid@3.3.18` override
  固定；开发工具链同时锁定已修复的 `fast-uri@3.1.5`、`hono@4.13.1`、
  `ip-address@10.5.0`、`js-yaml@4.3.1` 与 `undici@7.29.0`。
- 完整 `npm audit --audit-level=high` 与 production-only audit 均为 0。
- Go 构建基线升级到 `1.26.6`，修复 `net/url`、`html/template`、TLS、HTTP/2、
  XML/ASN.1 与 IDNA 标准库可达漏洞；gRPC 为 `1.82.1`，OpenTelemetry 家族为 `1.44.0`；
  `govulncheck` 报告 0 个可达漏洞。
- gitleaks 扫描全部 tracked 与未忽略 untracked 源文件，包括环境模板；忽略的本机
  `0600` 部署环境文件不再被 `dir` 模式误当成仓库内容。

## 运维与发布拓扑

- 预发布阶段只维护 `000001_initial_schema.up/down.sql`。
- 开发环境不使用 Docker；`scripts/dev.sh` 连接自行提供的 PostgreSQL、Redis、
  MinIO，并可按配置连接外部 Meilisearch。
- 生产 Compose 包含 PostgreSQL、Redis、MinIO、Meilisearch、Semantic Kernel、API、
  Worker、Web、CLI tools image、migrate 和 doctor；不包含 Nginx、OIDC 或 TLS 终结。
- 数据与应用服务继承只读根文件系统、非 root、capability drop 和
  `no-new-privileges` 策略。应用镜像在部署机本地按 `RELEASE_ID` 构建。
- `.gitattributes` 固定 Shell 与部署门禁文件为 LF，使 Windows+WSL 与 Linux CI
  执行同一套脚本。

## 2026-08-12 百科文章结构增量验证

- OpenAPI、领域服务、Worker 投影、导入 Composer 与 Web 阅读/编辑入口已完成
  References、Related topics、Related outlines、主 Entity 信息框和标准 See also 的
  跨层闭环；生成客户端由契约重新生成，未手改生成物。
- Go 全量测试、Web TypeScript 与 ESLint 已通过；渲染回归覆盖 Citation 全文统一
  编号、正文到 References 跳转以及多处引用回链。
- PostgreSQL 17 隔离空库执行初始化 Schema `up → down → up`，三次公共表数量为
  `83 → 1 → 83`；down 后仅保留 `schema_migrations`。
- 以上是本地实现与静态/隔离数据库验证；生产现有页面仍需部署后执行 Projection
  全量重建，并以真实导入页面完成浏览器和内容质量验收。

## 2026-07-31 验证快照

已通过：

- `go test ./...`、`go vet ./...`、`go build ./...`、`go mod verify`；
- CRLF/LF 等价的 gofmt 检查；
- `govulncheck@latest ./...` 与 CI 固定的 `govulncheck@v1.1.4 ./...`；
- `npm ci`/`npm install` 安装期适配、TypeScript、ESLint、Next production build；
- 完整 `npm audit` 与 production audit；
- OpenAPI Generator validate、客户端重新生成、Schema 嵌入副本一致性；
- Shell 语法、迁移文件规范、部署静态规则、Compose YAML 解析；
- 隔离空库执行唯一初始化迁移 `up → down → up`：每次 up 均得到 81 张表（含
  `schema_migrations`），down 后仅保留迁移元数据表；
- PostgreSQL 16、Redis、MinIO、Meilisearch、API、Linux Worker 与 Web 真实启动；
  页面发布/历史/Diff/回滚/改名/重定向、不可变快照、链接解析与反向链接、资产
  上传去重、Dataset/View、Component、Collection、知识图谱和治理 Apply 均完成
  API 联调；
- Meilisearch 关键词、语义与混合查询均使用真实索引和本地 HuggingFace embedder；
- 文本、PNG OCR、JSON 与 CSV 导入均经真实队列、对象存储、Worker、AI Gateway、
  Schema 校验、匹配与 Proposal 生成成功；PNG 使用真实 Tesseract，扫描 PDF 的
  Poppler + Tesseract 回退通过运行测试。重新打开浏览器后仍可从 `/imports` 找回
  Job、七阶段进度、运行记录、证据定位与 Proposal 链接；
- 全量 Projection 重建共扫描 4 页：3 页成功重建、1 个未发布页按设计跳过、
  0 失败；随后一致性检查 30/30 状态一致，Outbox backlog/dead、Projection
  error/stale 均为 0；
- 一条旧版缺少 `subject_entity_id` 的 `claim.changed` dead 事件通过兼容解码器
  回放成功；修复后的新 Claim Proposal Apply 两条事件均首轮完成；
- 非当前 Revision 经领域服务真实归档到 MinIO，API 从 cold tier 校验哈希后透明
  回源，AST 内容保持一致；
- 144/144 个 OpenAPI operationId 均由 Web 生成客户端调用路径引用，并由
  `make web-api-coverage` 验证调用文件可反向追踪到页面或全局 layout；Chrome
  headless 覆盖 34 个桌面路由、关键成功/空状态，以及真实 390×844 设备度量；
  窄屏 `html/body scrollWidth === clientWidth === 390`，斜杠标题的分段与编码
  URL 均可访问；
- 隔离远端全栈动态命中 144/144 个 OpenAPI operation handler，除故意指向
  `.invalid` 模型地址的 AI 连通性检查按契约返回 502 外，没有非预期 5xx；页面、
  Asset、Source/Citation、Dataset、Component、Collection、Entity/Claim/联邦、
  Proposal/Review/Apply、BulkReview、审计/保护/回滚、Import cancel/retry/upload、
  AI 配置脱敏、Revision 归档和双用户协作成功工作流均通过；
- Doctor 健康退出（0 error/critical；仅报告两条刻意未附 Citation 的测试 Claim
  证据警告）；
- gitleaks v8.28.0。

仍须在发布/目标环境补充：

- PostgreSQL/对象存储备份恢复演练和 RPO/RTO 记录；
- 10 万页面目标硬件容量、搜索语义质量和长时间队列/SLO 观察；
- 正式域名下的 HSTS、CSRF、账号恢复/MFA 与完整键盘、读屏、对比度人工验收。

仓库现有 Go 单元测试、149-operation handler 探针和高风险成功工作流仍不能穷举
全部 ProposalOperation 组合、外部模型供应商行为或容量/安全/人工可访问性；本轮真实
系统演练不能替代发布环境验收。

## 2026-08-18 Web/API/CLI 可操作性审计

- 权威 OpenAPI 当前共有 149 个 operation。147 个 Web-owned operation 全部通过生成
  客户端调用，调用文件均可沿 TypeScript import 图反向到达 47 个页面/global layout
  owner；`exchangeCLIAuthCode` 与 `revokeCurrentCLIToken` 是明确的 CLI transport。
- 69 个写操作分别落在注册/登录、页面编辑与工具、导入、资产、来源、Dataset、
  Component、Collection、Entity、联邦和治理工作区，均有表单、按钮或明确命令入口。
  动态详情页都有上游列表、搜索、审计、引用或关联对象链接。
- 正式域名批量验证 26 个非动态页面和 10 个真实数据动态页面，均返回 200；全局命令
  面板可发现主要知识、共建、治理和管理工作区。
- `/metrics` 是 Prometheus 抓取端点，不属于用户操作；协作 WebSocket 是编辑器内部
  传输协议，由 `/pages/[id]/edit` 使用并经 HTTPS/WSS E2E 覆盖。
- 新增 `scripts/check-web-api-coverage.mjs` 并纳入 `make check`。以后新增 operation
  没有前端调用，或调用只存在于未挂载组件中，提交门禁会失败。

## 2026-08-18 API 语义与工作流审计

- `TestAPIContractE2E` 从权威 OpenAPI 动态读取并命中 149/149 个 operation；所有
  handler 禁止非预期 5xx，只有配置为测试 `.invalid` 地址的 `testAIConfig`
  允许契约声明的 502/504 依赖失败。
- 成功工作流覆盖 Page/Revision/Diff/rollback/rename/redirect/BlockRedirect、
  section/anchor/projection、Asset、Source/Chunk/Citation、Dataset、Component、
  Manual/Rule Collection、Entity/Claim、联邦、Entity merge/rollback、Proposal
  preview/review/apply、BulkReview 全状态机、ChangeTag/audit/protection、
  ChangeBatch rollback、Import cancel/retry/upload、AI 配置与 Revision archive。
- 正式 Session + WebSocket 双用户测试继续覆盖 Presence、update 广播、幂等重放、
  stale sequence 409、当前 sequence 发布和 snapshot/compact 恢复。
- 修复 `create_entity` 的 Schema/运行时解码漂移：`language` 和 `description` 现在
  可通过严格 payload 解码，不再让 Schema 合法 Proposal 在 Apply 预检时误报 422。
- Parse stage 完成或恢复跳过时，Worker 现在在同一事务完成 stage 并写入
  `ImportJob.source_version_id`。即使后续模型调用失败，失败任务 API 仍可导航到
  已持久化 SourceVersion/Chunk，并从同一 checkpoint 重试。
- AI Trust 的更新成功路径依赖预先存在的 `ai/import` Actor；公开注册只创建 human，
  当前用户发起的导入 Proposal 也以 human 为作者，因此该策略尚未进入普通导入闭环，
  详见 [待解决问题](OutstandingIssues.md)。

## Agent CLI

- `backend/cmd/anby-wiki` 提供供 Agent 使用的 Go CLI；stdin/stdout 均为单一 JSON
  envelope，退出码区分成功、输入/契约错误和远端错误。
- CLI 从嵌入的权威 OpenAPI 读取 149 个 operationId，支持清单、搜索、Schema 描述和
  通用调用；发送前校验 path/query/header、JSON/multipart body，收到 JSON 后按状态码
  校验响应。文件上传读取本地路径，二进制响应统一转为 base64 JSON。
- Yjs 协作 WebSocket 不属于 OpenAPI，另由 `collaboration.run` 覆盖恢复、
  durable update、Presence 和 snapshot/compact，opaque bytes 均通过 base64。
- `/settings/cli` 使用浏览器 Session 生成十分钟一次性授权码，CLI 兑换只显示一次的
  Bearer Token。数据库只保存 SHA-256；后台显示前缀、状态、到期和最近使用时间并可撤销。
- CLI Token 映射到签发 Actor，不复制角色；每次请求实时读取 Actor/账号状态并继续经过
  现有 RBAC、PageProtection、Proposal Review/Apply 与领域服务边界。
- 详细协议与示例见 [Agent CLI](AgentCLI.md)。
- 隔离 PostgreSQL/Redis/Meilisearch 上的真实 E2E 已覆盖授权码兑换、Bearer Session、
  禁止 Bearer 递归签发、后台撤销、CLI 创建 Page、WorkingDocument 恢复和 CLI 自撤销。
- 初始化 Schema 在包含真实 Page 的隔离库上完成 `down → up`；同时修复了旧 down
  脚本在删除 Page 前提前删除 Namespace 种子导致的 FK 失败。

## 数据库状态

- 唯一迁移版本：`000001_initial_schema`。
- 初始化文件包含当前全部表、函数、触发器、约束、索引和固定种子；up/down 配对、
  命名与静态检查通过。
- 当前表和约束已在 PostgreSQL 17 隔离库重新执行 `up → down → up`；
  up 后 85 张表、down 后仅 `schema_migrations`，再次 up 结果一致。
- 首次生产上线后必须冻结版本 1，并恢复只增不改的增量迁移策略。
