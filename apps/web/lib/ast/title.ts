// 页面标题规范化（M2-T04）：前端镜像 backend/internal/page/title.go 的
// NormalizeTitle 语义（NFC → 空白折叠 → 小写），用于构造未解析 PageReference
// 的 normalized_title。两端保持一致，未解析引用才能在目标页面创建后被 Resolver 命中。
export function normalizeTitle(raw: string): string {
  return raw.normalize("NFC").replace(/\s+/gu, " ").trim().toLowerCase();
}
