# 当前实现状态

> 更新时间：2026-08-11
> 产品与能力审计依据：[整体设计方案](WikiDesignOnePage.md)

## 总体结论

`WikiDesignOnePage.md` 定义的模块化单体 API、独立 Worker、权威数据、治理写入、
可重建投影、知识与证据、导入、搜索、协作及平台化能力，均已落到代码与 HTTP
契约。适合用户操作或查看的后端能力均在 Web 中提供持久、可重新发现的入口；
导入队列、治理任务、审计活动和后台工具不再只存在于一次性流程页面。

当前代码已通过编译、契约、依赖安全和部署静态门禁，并在本机隔离环境完成
PostgreSQL、Redis、MinIO、Meilisearch、API、Linux Worker 与 Next.js Web 的真实
联调。“实现完成”仍不等于“生产发布就绪”：Docker/OCI 发布产物、目标规模容量、
正式域名安全边界与账号恢复等仍须在发布环境验收。详见
[待解决问题](OutstandingIssues.md)。

## 设计能力闭环

| 领域 | 当前实现 |
|---|---|
| 页面核心 | Wiki/Namespace、Page、不可变 Revision/ContentSnapshot、稳定 Block ID、历史、Diff、回滚、改名、PageRedirect、BlockRedirect、页面保护 |
| 知识图谱 | Entity/Property/Claim、标签/别名/主标签、Page 主 Entity 绑定、Claim 验证、Entity 合并与回滚、联邦 Wiki/Entity 映射、图谱查询 |
| 证据与媒体 | Source/Version/Chunk/Citation、Asset/AssetRevision、引用校验、反向使用、可审计来源与媒体目录 |
| 结构化内容 | Dataset/View/Record、Component/Version/信息框、静态与动态 Collection、成员维护与投影 |
| 治理 | ProposalOperation v1 全部 23 种 Operation、预览、风险、ReviewTask、冲突解决、批量审核、ChangeBatch、补偿回滚、审计事件、ChangeTag、AI Trust、事实一致性 |
| 导入与 AI | URL/HTML/文本/PDF/PNG/JPEG/JSON/CSV 获取，图片及扫描 PDF 的中英 OCR、结构化数据规范化、分阶段进度、幂等 Job、解析成功后的不可变恢复点与无重复来源的失败重试、Semantic Kernel 结构化调用与纠正重试、带页码/坐标/置信度的证据约束抽取、Proposal 生成、可持久查询的导入队列 |
| 投影与搜索 | Outbox 租约/重试/死信、链接/目录/锚点/章节/渲染/知识使用/组件依赖/图谱投影、PostgreSQL fallback、Meilisearch 关键词/混合/语义检索 |
| 规模与归档 | 章节懒加载、服务端可信 HTML 渲染、Revision 热冷分层与 S3 回源、Projection/Search 重建、容量基准命令 |
| 协作 | Yjs WorkingDocument、增量同步、Presence、发布换基、AI 三方合并、人工冲突决议 |
| 平台 | 本地账号/Session/RBAC、Redis 限流、安全头、OTel/Prometheus、备份恢复、Doctor、多 Wiki 读取隔离、生产部署清单 |

## 关键不变量

- Page、Revision、Entity、Claim、Source、Citation、Proposal 等权威状态只经领域
  Service 写入；Handler、Worker 和投影 Builder 不绕过服务拼接权威写入。
- Revision 与 ContentSnapshot 发布后不可变；链接、渲染、搜索、图谱和反向使用
  投影可删除并由 Outbox 重建。
- Proposal 的客户端合并只提供交互预览；服务端会按权威 AST 重新计算并拒绝不一致
  结果。23 种 Operation 的 Apply/rollback 都有领域语义。
- Page、History、Knowledge 和 Projection 的 UUID 读取路径校验当前 Wiki，避免跨
  Wiki ID 枚举泄漏；Source/Citation/Component 等共享目录的 Page 使用结果也按
  当前 Wiki 过滤。
- Claim 值按 Property 的完整 JSON Schema 校验；Entity label/alias 的语言、长度和
  alias type 同时有服务层与数据库约束。

## Web 信息架构

Web 使用桌面优先的现代百科 Shell：固定全局导航、命令面板、搜索、上下文侧栏、
目录与站点页脚。正文保留维基式可扫描排版，同时使用 shadcn/Radix、Lucide、
Sonner、Geist 和响应式卡片提升工具型页面的可读性。

持久入口包括：

- `/wiki/[...title]`、`/pages/[id]`、编辑、历史、Revision 详情、Diff 与页面工具；
  catch-all 路由原生支持标题中的 `/`，404 创建入口会完整保留标题；
- `/entities`、Entity/Claim/Citation 详情、知识图谱与联邦管理；
- `/sources`、`/assets`、Asset Revision、`/datasets`、`/components`、
  `/collections`；
- `/imports` 及每个 Job 的阶段、运行记录和进度详情；
- `/governance` 下的 Proposal 创建/详情、审核队列、批量审核、审计活动、页面保护、
  AI Trust、事实一致性与 Revision 存储；
- `/admin/ai` 的加密 Provider 密钥、模型、输出模式、超时、重试和连通性测试；
- `/explore` 的搜索工作台、反向链接和 `/explore/graph` 图谱工作台。

服务端数据统一通过 `apps/web/lib/api.ts` 创建的生成客户端访问，SWR 是唯一服务端
数据客户端缓存；Zustand 只保存本地交互状态。

## 搜索、渲染与存储

- `SearchAdapter` 保持产品无关接口。开发可用 PostgreSQL FTS；生产 Compose 默认
  `SEARCH_BACKEND=meilisearch`，内置固定版本 Meilisearch 服务。
- Worker 在 PostgreSQL 投影事务完成后提交 Meilisearch 文档并等待 task 终态；
  Page 行锁与 Current Revision 复核阻止旧 Revision 覆盖新索引。
- Meilisearch 支持关键词、facet/filter、高亮、混合与语义查询；本地
  HuggingFace embedder 默认不向第三方发送正文。
- 后端从已校验 AST 生成转义后的可信 HTML 和章节投影；阅读页可按章节懒加载，
  目录通过 MutationObserver 感知新章节并跟踪当前滚动位置。
- Revision snapshot 可由 Worker 归档到 S3，读取 API 对热/冷存储透明回源；
  归档状态、容量统计和手动归档在治理页面可见。

## 安全与依赖

- Web 使用 Next.js `16.2.12`；实际运行树固定到 PostCSS `8.5.25` 和 Sharp
  `0.35.3`。shadcn CLI 位于 devDependencies。
- 新披露的 `brace-expansion` 漏洞通过 `5.0.9` 修复；安装期幂等适配器只让
  遗留 `minimatch@3` 读取新版函数导出，未知版本会直接失败，避免“审计通过但
  ESLint 损坏”。
- 完整 `npm audit --audit-level=high` 与 production-only audit 均为 0。
- gRPC 已升级到 `1.82.1`，OpenTelemetry 家族统一到 `1.44.0`；
  `govulncheck` 报告 0 个可达漏洞。
- gitleaks 继续扫描全仓库，只精确豁免构建目录和两个字面部署占位值，不豁免整个
  环境模板。

## 运维与发布拓扑

- 预发布阶段只维护 `000001_initial_schema.up/down.sql`。
- 开发环境不使用 Docker；`scripts/dev.sh` 连接自行提供的 PostgreSQL、Redis、
  MinIO，并可按配置连接外部 Meilisearch。
- 生产 Compose 包含 PostgreSQL、Redis、MinIO、Meilisearch、Semantic Kernel、API、Worker、Web、
  migrate 和 doctor；不包含 Nginx、OIDC 或 TLS 终结。
- 数据与应用服务继承只读根文件系统、非 root、capability drop 和
  `no-new-privileges` 策略。应用镜像在部署机本地按 `RELEASE_ID` 构建。
- `.gitattributes` 固定 Shell 与部署门禁文件为 LF，使 Windows+WSL 与 Linux CI
  执行同一套脚本。

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
  Job、六阶段进度、运行记录、证据定位与 Proposal 链接；
- 全量 Projection 重建共扫描 4 页：3 页成功重建、1 个未发布页按设计跳过、
  0 失败；随后一致性检查 30/30 状态一致，Outbox backlog/dead、Projection
  error/stale 均为 0；
- 一条旧版缺少 `subject_entity_id` 的 `claim.changed` dead 事件通过兼容解码器
  回放成功；修复后的新 Claim Proposal Apply 两条事件均首轮完成；
- 非当前 Revision 经领域服务真实归档到 MinIO，API 从 cold tier 校验哈希后透明
  回源，AST 内容保持一致；
- 135/135 个 OpenAPI operationId 均由 Web 生成客户端调用路径引用；Chrome
  headless 覆盖 34 个桌面路由、关键成功/空状态，以及真实 390×844 设备度量；
  窄屏 `html/body scrollWidth === clientWidth === 390`，斜杠标题的分段与编码
  URL 均可访问；
- Doctor 健康退出（0 error/critical；仅报告两条刻意未附 Citation 的测试 Claim
  证据警告）；
- gitleaks v8.28.0。

仍须在发布/目标环境补充：

- Docker Compose 展开、四个 OCI target 构建、容器 healthcheck 与非 root 元数据
  验证（本机 Docker daemon 不可用）；
- 10 万页面目标硬件容量、搜索语义质量和长时间队列/SLO 观察；
- 正式域名下的 TLS、CSRF、账号恢复/MFA 与完整键盘、读屏、对比度人工验收。

仓库仍没有覆盖全部领域的完整自动化测试套件；现有 Go 单元/适配器/OCR 运行测试
与本轮真实系统演练不能替代发布环境的容量、安全和人工可访问性验收。

## 数据库状态

- 唯一迁移版本：`000001_initial_schema`。
- 初始化文件包含当前全部表、函数、触发器、约束、索引和固定种子；up/down 配对、
  命名与静态检查通过。
- 本次新增表和约束已在 PostgreSQL 16 隔离空库重新执行 `up → down → up`；
  up 后 81 张表、down 后仅 `schema_migrations`，再次 up 结果一致。
- 首次生产上线后必须冻结版本 1，并恢复只增不改的增量迁移策略。
