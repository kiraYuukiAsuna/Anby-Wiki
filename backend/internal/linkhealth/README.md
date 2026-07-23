# Link Health

M9-T05 外链健康检查由 Worker 常驻轮询，也可用
`go run ./cmd/worker -check-external-links` 单次执行一个批次。

## 安全边界

- 仅允许无用户信息的绝对 HTTP/HTTPS URL 和 80/443 端口。
- 初始校验、每次 Redirect 与实际 Dial 都重新解析 DNS；任一结果为私网、环回、
  链路本地、组播、CGNAT、文档、协议转换、benchmark 或其他保留地址即拒绝。
- HTTP 客户端禁用环境代理，最多跟随 5 次 Redirect，读取正文上限 64 KiB。
- 只保存有限内容摘要，不保存或记录响应正文、查询参数、凭据或 Token。

## 状态与调度

`external_resource.status` 使用既有 `unknown/ok/redirect/broken/blocked`：

- 2xx 为 `ok`；最终 URL 变化为 `redirect`；非 2xx 为 `broken`；
- SSRF 策略拒绝为 `blocked`；网络、TLS 或超时失败为 `broken`；
- `ok/redirect` 每 24 小时复查并清零连续失败；
- 首次网络、TLS 或超时失败在 5 分钟后短重试，后续失败进入常规退避；
- `broken/blocked` 从 1 小时开始指数退避，最大 7 天；
- Worker 用 `FOR UPDATE SKIP LOCKED` 领取并写入 15 分钟租约；每次领取轮换
  `lease_token`，完成与重排均以 token CAS，过期 Worker 不能覆盖新结果。

## 目标变化

Redirect 最终 URL 或 HTTP `Link: <...>; rel=canonical` 与原 URL 不同时，先通过
`ExternalResourceService.Upsert` 建立目标资源，再对当前 Revision 的每个
`external_link_usage` 创建一个幂等 `retarget_external_link` Proposal。
Operation 携带 Base Revision 与 Block hash，随后经 `ReviewService.Submit` 执行既有
风险策略；跨域替换进入人工 ReviewTask。检查本身不修改 AST、Page 或 Revision。
Proposal、Operation、Submit 分步持久化；重试会按幂等键从现有 draft 继续补齐，
编排失败则把仍持有租约的资源安排到 5 分钟后恢复。
