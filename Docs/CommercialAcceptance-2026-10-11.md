# 商业验收证据（2026-10-11）

## 结论

提交 `c1163d0` 已部署到 `https://anbywiki.momiya.cloud/`。复杂 PDF、扫描件、
多模态 Provider、不可变解析制品、ImportPlan、Proposal 治理、失败重试和 Worker
优雅重启已达到 Internal Beta 的商业使用门槛。

本结论不等于全产品 GA 授权。300 MiB 文件峰值、备份恢复、账号恢复/MFA、目标规模
搜索容量和人工可访问性仍按 `OutstandingIssues.md` 保留。

## 发布门禁

- GitHub Actions CI `38152894957`：backend、web、ai-kernel、contracts、
  client-drift、migrations、security、deploy 共 8 个 Job 全部通过。
- 本地：`go test ./...`、`make check` 通过；Docker 实例验证由 CI 与生产部署完成。
- 生产：Migration gate `current=1 expected=1 compatible=1..1 dirty=false`；
  部署前后及最终只读 Doctor 均为 0 issue。
- 当前 API/Worker/Web/AI Kernel 均为 `c1163d01d071` 且 healthy。

## 真实语料

| 场景 | 结果 | 关键证据 |
| --- | --- | --- |
| 三页真实论文，多栏/图/表 | 成功 | 七阶段成功；3 page、2 figure、1 table；3 个图像定位；质量 0.970；Proposal 已拒绝 |
| 原生文本与扫描页混合 PDF | 成功 | 七阶段成功；page/table/figure/text 齐全；质量 0.976；Proposal 已拒绝 |
| 纯扫描表格页 | 安全降级 | parse/extract/plan 成功；质量 0.690，进入 `action_required`；无 Proposal |
| 扫描页提示注入 | 安全拒绝 | SourceVersion 保留；Extraction `quality_gate`；无 Proposal |
| 截断但保留 PDF 签名 | 原子失败 | fetch 成功、parse `parse_failed`；无 SourceVersion/Proposal |
| 501 页合法 PDF | 边界拒绝 | 1.45 秒内 `pdf_page_limit_exceeded`；无逐页模型调用、SourceVersion 或 Proposal |

真实论文 SourceVersion 使用 `hybrid` track、`anby-vision-pdf` parser 和 Prompt
version 2。越界 locator 被裁剪或省略并写入安全 warning，正文、表格和图片 Chunk
仍完整持久化。

## 故障与治理

- 上传幂等冲突：相同 Idempotency-Key、不同文件内容返回 409；当前 Asset Revision
  哈希前后一致，未创建替代 Job。
- AI Kernel 不可用：首轮以 `extraction_provider_failed` 失败，但已解析
  SourceVersion 可见；恢复后重试同一 Job，fetch/parse 均为 `skipped`，复用相同
  SourceVersion 后完成规划，测试 Proposal/Job 已清理。
- Worker restart：复杂 PDF 在 `parse` 阶段收到 Compose restart；Worker 等待 55 秒
  完成在途任务后退出并健康重启。Job 只有一个 Run，没有 `worker_interrupted`。
- 测试生成的 Proposal 共 5 个，最终全部为 `rejected`；没有测试内容被批准或应用。
- Outbox backlog/dead、Projection error 均为 0。

## 资源证据

三页并发视觉分析期间采样 48 次，峰值常驻内存：

| 服务 | 峰值 |
| --- | ---: |
| API | 145.2 MiB |
| AI Kernel | 119.7 MiB |
| Web | 45.3 MiB |
| Worker | 20.2 MiB |

同一真实论文在不同模型运行中既出现过自动通过，也出现过质量 0.690 的
`action_required`。系统没有降低 0.70 门槛或发布低质量 Proposal；这种供应商波动
按人工复核路径显式暴露。

## 保留风险

1. 尚未执行 300 MiB 文件与多任务并发的峰值内存、耗时和供应商成本测试。
2. 真实供应商超时/限流仍需结合账单与告警验证；本轮已验证内部 Kernel 不可用、
   错误分类、恢复和重试。
3. 全产品商业发布仍受备份恢复、账号恢复/MFA、目标规模搜索容量及人工可访问性阻塞。
