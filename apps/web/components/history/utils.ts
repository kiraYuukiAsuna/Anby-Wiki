// 历史/回滚界面的共用展示辅助（M2-T05）。

import { compactId } from "@/lib/display-id";

/** UUID 短 id 展示（保留随机尾部，完整值放 title/aria）。 */
export function shortId(id: string): string {
  return compactId(id);
}

/** 本地化日期时间（与 components/revision-info.tsx 一致的格式）。 */
export function formatDateTime(date: Date): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export interface RevisionActorInfo {
  actorId: string;
  actorUsername?: string;
  actorEmail?: string;
  actorDisplayName?: string;
}

export interface RevisionTimeInfo {
  id: string;
  createdAt: Date;
}

export function revisionLabel(revision: RevisionTimeInfo): string {
  return `发布于 ${formatDateTime(revision.createdAt)}`;
}

export function revisionAuthorLabel(revision: RevisionActorInfo): string {
  const username = revision.actorUsername?.trim();
  if (username) return `@${username}`;
  const displayName = revision.actorDisplayName?.trim();
  if (displayName) return displayName;
  return "未知编辑者";
}

export function revisionAuthorMeta(revision: RevisionActorInfo): string | null {
  const email = revision.actorEmail?.trim();
  if (email) return email;
  const username = revision.actorUsername?.trim();
  const displayName = revision.actorDisplayName?.trim();
  if (displayName && displayName !== username) return displayName;
  return null;
}

/** 索引路径展示：[0.2.1]，空路径（顶层）为 [0] 形式原样展示。 */
export function formatPath(path?: number[]): string {
  if (!path) return "—";
  return `[${path.join(".")}]`;
}
