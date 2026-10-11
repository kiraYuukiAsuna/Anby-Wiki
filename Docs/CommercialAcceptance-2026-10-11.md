# 商业验收证据（2026-10-11）

## 结论

提交 `18944a7d0c30` 已部署到 `https://anbywiki.momiya.cloud/`。复杂 PDF、扫描件、
多模态 Provider、不可变解析制品、ImportPlan、Proposal 治理、失败重试和 Worker
优雅重启已达到 Internal Beta 的商业使用门槛。

本结论不等于全产品 GA 授权。298 MiB 完整 Worker 路径、单实例并发背压和内存回收
已达标；持续压力、多副本策略、备份恢复、账号恢复/MFA、目标规模搜索容量和人工
可访问性仍按 `OutstandingIssues.md` 保留。

## 发布门禁

- GitHub Actions CI `38173395650`：backend、web、ai-kernel、contracts、
  client-drift、migrations、security、deploy 共 8 个 Job 全部通过。
- 本地：`go test ./...`、`make check` 通过；Docker 实例验证由 CI 与生产部署完成。
- 生产：Migration gate `current=1 expected=1 compatible=1..1 dirty=false`；
  部署前后及最终只读 Doctor 均为 0 issue。
- 当前 API/Worker/Web/AI Kernel 均为 `18944a7d0c30` 且 healthy。

## 真实语料

| 场景 | 结果 | 关键证据 |
| --- | --- | --- |
| 三页真实论文，多栏/图/表 | 成功 | 七阶段成功；3 page、2 figure、1 table；3 个图像定位；质量 0.970；Proposal 已拒绝 |
| 原生文本与扫描页混合 PDF | 成功 | 七阶段成功；page/table/figure/text 齐全；质量 0.976；Proposal 已拒绝 |
| 三份关联文档正序合并 | 安全降级 | 9 页、26 Chunk、19 个区域定位；恰好规划 3 个目标页面；质量 0.690；无 Proposal |
| 同一关联文档逆序合并 | 安全降级 | Chunk 与路由形态不变；暴露并修复顺序敏感关系误判；质量 0.690；无 Proposal |
| 正序+逆序关联文档拼接 | 安全降级 | 18 页、50 Chunk、36 个区域定位；覆盖第 1–18 页；仍恰好规划 3 个目标页面；质量 0.690；无 Proposal |
| 纯扫描表格页 | 安全降级 | parse/extract/plan 成功；质量 0.690，进入 `action_required`；无 Proposal |
| 扫描页提示注入 | 安全拒绝 | SourceVersion 保留；Extraction `quality_gate`；无 Proposal |
| 截断但保留 PDF 签名 | 原子失败 | fetch 成功、parse `parse_failed`；无 SourceVersion/Proposal |
| 501 页合法 PDF | 边界拒绝 | 1.45 秒内 `pdf_page_limit_exceeded`；无逐页模型调用、SourceVersion 或 Proposal |

真实论文 SourceVersion 使用 `hybrid` track、`anby-vision-pdf` parser 和 Prompt
version 2。越界 locator 被裁剪或省略并写入安全 warning，正文、表格和图片 Chunk
仍完整持久化。

## 关联文档与顺序扰动

固定语料由架构规范、运维手册和事故报告组成，每份 3 页，共 9 页。语料混合原生文本、
扫描页、表格、图表、文档编号、日期冲突及跨文档引用，同时包含以下真假关系：

- `AG-INC-042 updates AG-OPS-2026`；
- `AG-OPS-2026 updates and obsoletes AG-OPS-2025`；
- `AG-OPS-2026 operationalizes AG-ARCH-2026`，但不更新架构规范；
- `AG-ARCH-2026 remains authoritative`，不能被误读为事故报告更新架构规范。

正序 Job `01a12bd2-f0c2-7c1b-b573-5fb2cc2dc812` 与逆序 Job
`01a12bd4-8c55-7987-8e46-c33fa29e84f6` 均通过 fetch、parse、extract、plan，
稳定得到 7 page、8 table、3 figure、2 chart、6 text Chunk，覆盖第 1 至 9 页，
并只生成 `AG-ARCH-2026`、`AG-OPS-2026`、`AG-INC-042` 三个 create route。

逆序语料曾让模型把 `operationalizes` 和 `remains authoritative` 误写为 `updates`。
服务端现在只接受由主语、方向标记和值直接连接的关系证据；对 “It updates and
obsoletes ...” 仅在同一 Chunk 内存在 240 字符以内、无其他作品插入的唯一前置主语时
放行。不可变历史 Extraction 不重写，读取时重新执行该确定性守卫。

ImportPlan 的 `related_to` 只表示面向已存在 Page 的 link route，不表示候选 Claim
或三条同时创建的页面之间的全部语义关系。因此当前无已发布目标页时 `related_to=[]`
是契约语义，不是关联抽取失败。

同一正序与逆序 dossier 继续拼接为 18 页后，生产 Job
`01a12c79-81a2-779b-b829-188398c2b7b4` 在 111.765 秒内完成 fetch、parse、
extract 和 plan，生成 14 page、15 table、6 figure、3 chart、12 text，共 50 个
Chunk；36 个 Chunk 带页内区域，页码完整覆盖 1–18。计划仍只有三个 create route，
没有因重复和顺序反转扩张目标页面。

## 故障与治理

- 上传幂等冲突：相同 Idempotency-Key、不同文件内容返回 409；当前 Asset Revision
  哈希前后一致，未创建替代 Job。
- AI Kernel 不可用：首轮以 `extraction_provider_failed` 失败，但已解析
  SourceVersion 可见；恢复后重试同一 Job，fetch/parse 均为 `skipped`，复用相同
  SourceVersion 后完成规划，测试 Proposal/Job 已清理。
- Worker restart：复杂 PDF 在 `parse` 阶段收到 Compose restart；Worker 等待 55 秒
  完成在途任务后退出并健康重启。Job 只有一个 Run，没有 `worker_interrupted`。
- 测试生成的 Proposal 共 5 个，最终全部为 `rejected`；没有测试内容被批准或应用。
- 本轮新增的关联文档任务均在 `action_required` 后取消，没有生成 Proposal。
- 初次 128 MiB 上传经 Next.js 外部 rewrite 时触发其默认 10 MiB 缓冲截断：Go API
  收到部分请求并返回 413，但客户端在约 30.38 秒后得到非 JSON 500。生产入口现由
  Nginx 把 `/api/*`、`/healthz`、`/readyz` 直接转发到仅绑定回环地址的 Go API，
  `proxy_request_buffering off`；Next 的 301 MiB 设置只保留为兼容路径。
- 修复后 7,474 字节 PDF 经同源 Session 和 multipart 返回 201，Job 从 `queued`
  取消为 `cancelled`，无 Proposal；证明直连入口不破坏 cookie、上传或取消语义。
- 大文件预检进一步改为 1 MiB 内存 + 受控磁盘临时文件，MIME、恶意签名与 SHA-256
  流式执行；Evidence 回卷后直传对象存储，Worker 按对象 `Head` 精确分配一份内容。
  同一 API 进程同时只接纳一个上传；298 MiB 在途时第二个请求立即返回结构化 503，
  主请求不受影响且临时目录最终为空。
- 首次部署因受限 `storage-init` 在 `chown` 后无权执行冗余 `chmod` 而安全中止，
  自动恢复旧版本；移除该命令后重新部署成功，目录保持 `0700` 和 `10001:10001`。
- 最终 active ImportJob、开放导入 Proposal、Outbox backlog/dead、Projection error
  均为 0。

## 资源证据

三页并发视觉分析期间采样 48 次，峰值常驻内存：

| 服务 | 峰值 |
| --- | ---: |
| API | 145.2 MiB |
| AI Kernel | 119.7 MiB |
| Web | 45.3 MiB |
| Worker | 20.2 MiB |

18 页关联 dossier 处理期间的容器峰值为 API 86.77 MiB、Worker 31.5 MiB、
AI Kernel 126.7 MiB、Web 49.17 MiB。

修复前，两个带真实 EmbeddedFile 对象且经生产 Worker 镜像 `pdfinfo`/`pdfdetach`
验证的大 PDF 使用 256 字符标题在存储前触发确定性 422：

| 文件 | HTTP 结果 | API cgroup 峰值 | Web 峰值 | 宿主机最低可用内存 |
| --- | --- | ---: | ---: | ---: |
| 134,218,424 bytes（128 MiB 级） | 422 JSON，3.973 秒 | 826.7 MiB | 39.3 MiB | 3.27 GiB |
| 312,472,266 bytes（298 MiB 级） | 422 JSON，6.059 秒 | 1,905.6 MiB | 39.4 MiB | 2.17 GiB |

两次请求前后 Asset/ImportJob/Proposal/活跃 Outbox 计数均为 `30/30/5/0`，API
未重启、未 OOM 且始终 healthy。298 MiB 请求后 API 仍保留约 1.86 GiB cgroup
内存，测试结束后通过单实例重启恢复到 8.5 MiB；这说明当前全量 multipart、
`io.ReadAll` 和签名扫描会产生显著堆保留，不能只依据“未 OOM”判定容量达标。

流式化后使用同类 PDF 重新测试：

| 场景 | 结果 | API 总峰值 | API anon 峰值 | Worker 峰值 | 宿主机最低可用内存 |
| --- | --- | ---: | ---: | ---: | ---: |
| 134,214,531 bytes，存储前 422 | 1.441 秒，权威计数不变 | 211.4 MiB | 75.9 MiB | 14.9 MiB | 3.86 GiB |
| 312,472,451 bytes，存储前 422 | 2.650 秒，权威计数不变 | 390.5 MiB | 80.0 MiB | 11.1 MiB | 3.87 GiB |
| 312,472,451 bytes，完整 Worker | 20 秒，七阶段成功、质量 1.0 | 461.2 MiB | 148.3 MiB | 774.0 MiB | 2.96 GiB |

API 总峰值主要包含可回收的临时文件页缓存；两次 422 后 anon 只增加约 4 MiB，
临时目录均回到 0。完整 Job `01a12cbc-65ea-7c7a-9ae6-9d67c2336849` 生成一个 page
Chunk，review 为 `skipped` 且无 Proposal；30 秒后 API/Worker 分别回落到
150.3/312.8 MiB，无需重启。全程无 OOM 或服务重启，最终 Doctor 和活动
Job/Proposal/Outbox/Dead 均为 0。

同一真实论文在不同模型运行中既出现过自动通过，也出现过质量 0.690 的
`action_required`。系统没有降低 0.70 门槛或发布低质量 Proposal；这种供应商波动
按人工复核路径显式暴露。

## 保留风险

1. 单次 298 MiB 完整链路和并发拒绝已通过；仍需做持续上传/重试压力、异常退出后的
   临时文件清理演练，以及未来多 API/Worker 副本下的全局并发策略。
2. 真实供应商超时/限流仍需结合账单与告警验证；本轮已验证内部 Kernel 不可用、
   错误分类、恢复和重试。
3. 当前关联回归集已扩展到 18 页；还需补充长文档跨页表格、更多语言和模型升级后的
   稳定性抽样。
4. 全产品商业发布仍受备份恢复、账号恢复/MFA、目标规模搜索容量及人工可访问性阻塞。
