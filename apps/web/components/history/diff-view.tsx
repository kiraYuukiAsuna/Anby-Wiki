// 结构 Diff 视图（M2-T05）：/pages/[id]/diff 的展示主体（纯渲染，无客户端交互）。
//
// 按 DocumentDiff.changes 展示四类块级变更：
// added（绿）/ removed（红）/ changed（黄，含字段级 before/after）/ moved（蓝，before→after path）。
// 块定位显示 block 短 id（完整 id 放 title）与父块短 id；changes 为空（from==to）显示「两版相同」。
import type {
  BlockChange,
  DocumentDiff,
} from "../../../../contracts/generated/typescript";

import { formatPath, shortId } from "./utils";

const CHANGE_STYLES: Record<
  BlockChange["type"],
  { label: string; badge: string; row: string }
> = {
  added: {
    label: "新增",
    badge: "bg-green-500/10 text-green-700",
    row: "border-l-green-500",
  },
  removed: {
    label: "删除",
    badge: "bg-red-500/10 text-red-700",
    row: "border-l-red-500",
  },
  changed: {
    label: "修改",
    badge: "bg-amber-500/10 text-amber-700",
    row: "border-l-amber-500",
  },
  moved: {
    label: "移动",
    badge: "bg-blue-500/10 text-blue-700",
    row: "border-l-blue-500",
  },
};

function DiffChangeRow({ change }: { change: BlockChange }) {
  const style = CHANGE_STYLES[change.type];
  return (
    <li
      data-change-type={change.type}
      className={`rounded-r-lg border border-l-4 border-border px-3 py-2 ${style.row}`}
    >
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
        <span
          className={`rounded-full px-2 py-0.5 text-xs font-medium ${style.badge}`}
        >
          {style.label}
        </span>
        <span title={change.blockId}>
          块 <code className="font-mono">{shortId(change.blockId)}</code>
        </span>
        {change.parentId ? (
          <span
            className="text-xs text-muted-foreground"
            title={change.parentId}
          >
            父块 {shortId(change.parentId)}
          </span>
        ) : (
          <span className="text-xs text-muted-foreground">顶层块</span>
        )}
        {change.type === "added" || change.type === "removed" ? (
          <span className="text-xs text-muted-foreground">
            路径 {formatPath(change.path)}
          </span>
        ) : null}
        {change.type === "moved" ? (
          <span className="text-xs text-muted-foreground">
            路径 {formatPath(change.beforePath)} → {formatPath(change.afterPath)}
          </span>
        ) : null}
      </div>
      {change.type === "changed" && change.fields ? (
        <dl className="mt-2 flex flex-col gap-1">
          {change.fields.map((field) => (
            <div
              key={field.field}
              className="flex flex-wrap items-baseline gap-x-2 text-xs"
            >
              <dt className="font-mono font-medium">{field.field}</dt>
              <dd className="min-w-0">
                <code className="rounded bg-red-500/10 px-1 py-0.5 font-mono break-all text-red-700">
                  {field.before}
                </code>
                <span className="mx-1 text-muted-foreground">→</span>
                <code className="rounded bg-green-500/10 px-1 py-0.5 font-mono break-all text-green-700">
                  {field.after}
                </code>
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
    </li>
  );
}

export function DiffView({ diff }: { diff: DocumentDiff }) {
  if (diff.changes.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
        两版相同，没有结构差异。
      </p>
    );
  }
  return (
    <ol className="flex flex-col gap-2">
      {diff.changes.map((change, index) => (
        <DiffChangeRow
          key={`${change.blockId}-${change.type}-${index}`}
          change={change}
        />
      ))}
    </ol>
  );
}
