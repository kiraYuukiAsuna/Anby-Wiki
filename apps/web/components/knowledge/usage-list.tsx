import Link from "next/link";

import type { ReferenceUsage } from "../../../../contracts/generated/typescript";

import { compactId } from "@/lib/display-id";

export type ReferenceUsagePageGroup = {
  pageId: string;
  pageTitle: string;
  revisionId: string;
  items: ReferenceUsage[];
  blockCount: number;
  mentionTexts: string[];
  claimIds: string[];
};

export function groupReferenceUsagesByPage(
  items: ReferenceUsage[],
): ReferenceUsagePageGroup[] {
  const groups = new Map<
    string,
    ReferenceUsagePageGroup & {
      blockIds: Set<string>;
      mentionSet: Set<string>;
      claimIdSet: Set<string>;
    }
  >();

  for (const item of items) {
    let group = groups.get(item.pageId);
    if (!group) {
      group = {
        pageId: item.pageId,
        pageTitle: item.pageTitle,
        revisionId: item.revisionId,
        items: [],
        blockCount: 0,
        mentionTexts: [],
        claimIds: [],
        blockIds: new Set<string>(),
        mentionSet: new Set<string>(),
        claimIdSet: new Set<string>(),
      };
      groups.set(item.pageId, group);
    }

    group.items.push(item);
    group.blockIds.add(item.blockId);
    const mentionText = item.mentionText?.trim();
    if (mentionText) group.mentionSet.add(mentionText);
    if (item.claimId) group.claimIdSet.add(item.claimId);
  }

  return Array.from(groups.values(), (group) => ({
    pageId: group.pageId,
    pageTitle: group.pageTitle,
    revisionId: group.revisionId,
    items: group.items,
    blockCount: group.blockIds.size,
    mentionTexts: Array.from(group.mentionSet),
    claimIds: Array.from(group.claimIdSet),
  }));
}

export function PageGroupedUsageList({
  groups,
  occurrenceLabel,
  showClaimContexts = false,
}: {
  groups: ReferenceUsagePageGroup[];
  occurrenceLabel: "提及" | "引用";
  showClaimContexts?: boolean;
}) {
  if (groups.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        暂无当前 Revision 的页面{occurrenceLabel}；投影异步更新时可能短暂为空。
      </p>
    );
  }

  return (
    <ul className="divide-y divide-border">
      {groups.map((group) => (
        <li key={group.pageId} className="py-3 first:pt-0 last:pb-0">
          <Link
            href={`/pages/${group.pageId}#${group.items[0].blockId}`}
            className="font-medium text-blue-600 hover:underline"
          >
            {group.pageTitle}
          </Link>
          <p className="mt-1 text-xs text-muted-foreground">
            Revision{" "}
            <span title={group.revisionId}>{compactId(group.revisionId)}</span> ·{" "}
            {group.items.length} 处{occurrenceLabel} · {group.blockCount} 个区块
          </p>
          {group.mentionTexts.length > 0 ? (
            <p className="mt-1 break-words text-xs text-muted-foreground">
              展示文本：{group.mentionTexts.map((text) => `“${text}”`).join("、")}
            </p>
          ) : null}
          {showClaimContexts && group.claimIds.length > 0 ? (
            <p className="mt-1 text-xs text-muted-foreground">
              Claim 上下文：
              {group.claimIds.map((claimId, index) => (
                <span key={claimId}>
                  {index > 0 ? "、" : null}
                  <Link
                    href={`/claims/${claimId}`}
                    title={claimId}
                    className="text-blue-600 hover:underline"
                  >
                    {compactId(claimId)}
                  </Link>
                </span>
              ))}
            </p>
          ) : null}
          {group.items.length > 1 ? (
            <details className="mt-2 text-xs text-muted-foreground">
              <summary className="cursor-pointer select-none hover:text-foreground">
                查看 {group.items.length} 个精确位置
              </summary>
              <ul className="mt-2 space-y-1.5 border-l pl-3">
                {group.items.map((item) => (
                  <li key={`${item.blockId}:${item.nodeId}:${item.claimId ?? ""}`}>
                    <Link
                      href={`/pages/${item.pageId}#${item.blockId}`}
                      className="hover:text-foreground hover:underline"
                    >
                      Block{" "}
                      <span title={item.blockId}>{compactId(item.blockId)}</span> · Node{" "}
                      {item.nodeId}
                      {item.mentionText ? ` · “${item.mentionText}”` : ""}
                    </Link>
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
