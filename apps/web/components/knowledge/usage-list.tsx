import Link from "next/link";

import type { ReferenceUsage } from "../../../../contracts/generated/typescript";

function shortId(id: string): string {
  return id.slice(0, 8);
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
