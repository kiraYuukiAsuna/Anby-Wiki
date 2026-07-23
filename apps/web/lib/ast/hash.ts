// AST v1 内容哈希：SHA-256(canonicalJson) 的小写 hex。
//
// 与 backend/internal/ast.ContentHash 对同一文档产出相同哈希。
// 使用 node:crypto（同步 API）；仅在 Node 环境（测试 / 服务端）可用，
// 浏览器端如需哈希请改用 Web Crypto（异步），不要在客户端组件中引入本模块。
import { createHash } from "node:crypto";

import { canonicalJson } from "./serialize";

/** contentHash 返回 SHA-256(canonicalJson(value)) 的小写 hex 字符串。 */
export function contentHash(value: unknown): string {
  return createHash("sha256").update(canonicalJson(value), "utf8").digest("hex");
}
