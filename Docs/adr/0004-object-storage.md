# ADR-0004：对象存储

状态：已接受（M0-T02）
日期：2026-07-21

## 背景

原始来源、PDF、图片和抓取产物需要对象存储；本地开发与 CI 需要可复现环境。

## 决策

- 接口：**S3 兼容 API**，Go 侧使用 `aws-sdk-go-v2` 的 S3 client（通过 `endpoint` 指向任意 S3 兼容实现）。
- 本地/CI：**MinIO**（docker-compose 服务，含初始化 bucket 的 init 容器）。
- 生产：任何 S3 兼容服务；配置项仅有 endpoint、region、bucket、凭据，不出现供应商专有特性。
- 业务代码只依赖 `internal/platform/storage` 定义的窄接口（Put/Get/Head/Delete/PresignGet），不直接依赖 AWS SDK 类型。
- 对象键约定：`{env}/{domain}/{content_hash前2位}/{content_hash}`，内容寻址去重（M4-T04 落地）。

## 备选方案

- 文件系统存储：无法平滑演进到生产，排除。
- SeaweedFS 等自建：P0 无必要。

## 影响

- `infra/local/docker-compose.yml` 含 MinIO 服务与 bucket 初始化。
- M6 的恶意文件隔离通过独立 bucket + 安全扫描流程实现，扩展点保留。
