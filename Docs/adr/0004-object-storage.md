# ADR-0004：对象存储

状态：已接受（S3 接口保持；本地/生产实现已修订）
日期：2026-07-21

## 背景

原始来源、PDF、图片和抓取产物需要对象存储；本地开发与 CI 需要可复现环境。

## 决策

- 接口：**S3 兼容 API**，Go 侧使用 `aws-sdk-go-v2` 的 S3 client（通过 `endpoint` 指向任意 S3 兼容实现）。
- 本地开发：使用操作者自行提供的 S3 兼容服务，不由仓库拉起 Docker。
- 生产：固定版本 RustFS；配置项仍只有 endpoint、region、bucket、凭据，不使用
  供应商专有业务能力。一次性初始化通过官方 AWS CLI 以 path-style 创建私有 bucket。
- 业务代码只依赖 `internal/platform/storage` 定义的窄接口（Put/Get/Head/Delete/PresignGet），不直接依赖 AWS SDK 类型。
- 对象键约定：`{env}/{domain}/{content_hash前2位}/{content_hash}`，内容寻址去重（M4-T04 落地）。

## 备选方案

- 文件系统存储：无法平滑演进到生产，排除。
- SeaweedFS 等自建：P0 无必要。

## 影响

- 仓库不再维护 `infra/local`。生产 RustFS 使用根目录 `data/rustfs/` 持久化；
  历史 MinIO 数据必须经 S3 导出/恢复，不能直接复制底层数据目录。
- M6 的恶意文件隔离通过独立 bucket + 安全扫描流程实现，扩展点保留。
