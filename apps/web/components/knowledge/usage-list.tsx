import Link from "next/link";

import type { ReferenceUsage } from "../../../../contracts/generated/typescript";

function shortId(id: string): string {
  return id.slice(0, 8);
}

function shortLocator(id: string): string {
  return `${id.slice(0, 4)}…${id.slice(-4)}`;
}

export type ReferenceUsagePageGroup = {
  pageId: string;
  pageTitle: string;
  revisionId: string;
  items: ReferenceUsage[];
  blockCount: number;
  mentionTexts: string[];
};

export function groupReferenceUsagesByPage(
  items: ReferenceUsage[],
): ReferenceUsagePageGroup[] {
  const groups = new Map<
    string,
    ReferenceUsagePageGroup & { blockIds: Set<string>; mentionSet: Set<string> }
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
        blockIds: new Set<string>(),
        mentionSet: new Set<string>(),
      };
      groups.set(item.pageId, group);
    }

    group.items.push(item);
    group.blockIds.add(item.blockId);
    const mentionText = item.mentionText?.trim();
    if (mentionText) group.mentionSet.add(mentionText);
  }

  return Array.from(groups.values(), (group) => ({
    pageId: group.pageId,
    pageTitle: group.pageTitle,
    revisionId: group.revisionId,
    items: group.items,
    blockCount: group.blockIds.size,
    mentionTexts: Array.from(group.mentionSet),
  }));
}

export function UsageList({ items }: { items: ReferenceUsage[] }) {
  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        暂无当前 Revision 的页面使用位置；投影异步更新时可能短暂为空。
      </p>
    );
  }

  return (
    <ul className="divide-y divide-border">
      {items.map((item) => (
        <li
          key={`${item.pageId}:${item.blockId}:${item.nodeId}`}
          className="py-3 first:pt-0 last:pb-0"
        >
          <Link
            href={`/pages/${item.pageId}#${item.blockId}`}
            className="font-medium text-blue-600 hover:underline"
          >
            {item.pageTitle}
          </Link>
          <p className="mt-1 text-xs text-muted-foreground">
            Revision {shortId(item.revisionId)} · Block {shortId(item.blockId)} · Node {item.nodeId}
            {item.mentionText ? ` · “${item.mentionText}”` : ""}
          </p>
        </li>
      ))}
    </ul>
  );
}

export function PageGroupedUsageList({
  groups,
}: {
  groups: ReferenceUsagePageGroup[];
}) {
  if (groups.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        暂无当前 Revision 的页面提及；投影异步更新时可能短暂为空。
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
            Revision {shortId(group.revisionId)} · {group.items.length} 处提及 ·{" "}
            {group.blockCount} 个区块
          </p>
          {group.mentionTexts.length > 0 ? (
            <p className="mt-1 break-words text-xs text-muted-foreground">
              展示文本：{group.mentionTexts.map((text) => `“${text}”`).join("、")}
            </p>
          ) : null}
          {group.items.length > 1 ? (
            <details className="mt-2 text-xs text-muted-foreground">
              <summary className="cursor-pointer select-none hover:text-foreground">
                查看 {group.items.length} 个位置
              </summary>
              <ul className="mt-2 space-y-1.5 border-l pl-3">
                {group.items.map((item) => (
                  <li key={`${item.blockId}:${item.nodeId}`}>
                    <Link
                      href={`/pages/${item.pageId}#${item.blockId}`}
                      className="hover:text-foreground hover:underline"
                    >
                      Block {shortLocator(item.blockId)} · Node {item.nodeId}
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
