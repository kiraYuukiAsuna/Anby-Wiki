// 历史/回滚界面的共用展示辅助（M2-T05）。

/** UUID 短 id 展示（取前 8 位，完整值放 title/aria）。 */
export function shortId(id: string): string {
  return id.slice(0, 8);
}

/** 本地化日期时间（与 components/revision-info.tsx 一致的格式）。 */
export function formatDateTime(date: Date): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

/** 索引路径展示：[0.2.1]，空路径（顶层）为 [0] 形式原样展示。 */
export function formatPath(path?: number[]): string {
  if (!path) return "—";
  return `[${path.join(".")}]`;
}
