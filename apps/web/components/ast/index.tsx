// Typed Block AST v1 → React 的原生渲染器（编辑预览、历史与投影降级路径）。
// 当前阅读页优先消费版本化 RenderedPage HTML，确保 Component/Claim 动态值与
// Worker 投影保持一致；AST 渲染器仍是不可变快照的客户端安全兜底。
import Link from "next/link";
import {
  AtSign,
  Database,
  ExternalLink,
  Info,
  OctagonAlert,
  Puzzle,
  TriangleAlert,
} from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/utils";
import type {
  Block,
  CalloutKind,
  Document,
  InlineNode,
  Mark,
  TextNode,
} from "@/lib/ast/schema";
import { safeHttpUrl } from "@/lib/http-url";
import { AssetImage, AssetVideo } from "@/components/ast/asset-media";

const HEADING_STYLES: Record<number, string> = {
  1: "mt-8 mb-4 text-3xl font-bold tracking-tight",
  2: "mt-8 mb-3 text-2xl font-semibold tracking-tight",
  3: "mt-6 mb-3 text-xl font-semibold",
  4: "mt-6 mb-2 text-lg font-semibold",
  5: "mt-4 mb-2 text-base font-semibold",
  6: "mt-4 mb-2 text-sm font-semibold text-muted-foreground",
};

type CitationNumbers = ReadonlyMap<string, number>;

function HeadingView({
  block,
  citationNumbers,
}: {
  block: Extract<Block, { type: "heading" }>;
  citationNumbers: CitationNumbers;
}) {
  const level = Math.min(6, Math.max(1, block.level));
  const Tag = `h${level}` as "h1";
  return (
    <Tag
      id={block.id}
      className={cn(
        "scroll-mt-20 border-b border-transparent",
        HEADING_STYLES[level],
      )}
    >
      <InlineNodes nodes={block.content} citationNumbers={citationNumbers} />
    </Tag>
  );
}

const CALLOUT_STYLES: Record<
  CalloutKind,
  { className: string; Icon: typeof Info; label: string }
> = {
  info: {
    className: "border-blue-500/40 bg-blue-500/10 text-foreground",
    Icon: Info,
    label: "信息",
  },
  warning: {
    className: "border-amber-500/40 bg-amber-500/10 text-foreground",
    Icon: TriangleAlert,
    label: "警告",
  },
  danger: {
    className: "border-red-500/40 bg-red-500/10 text-foreground",
    Icon: OctagonAlert,
    label: "危险",
  },
};

/** 渲染单个 Block（容器递归）。 */
export function BlockView({
  block,
  citationNumbers = new Map(),
}: {
  block: Block;
  citationNumbers?: CitationNumbers;
}): ReactNode {
  switch (block.type) {
    case "heading":
      return <HeadingView block={block} citationNumbers={citationNumbers} />;
    case "paragraph":
      return (
        <p className="my-3 leading-7">
          <InlineNodes nodes={block.content} citationNumbers={citationNumbers} />
        </p>
      );
    case "bullet_list":
      return (
        <ul className="my-3 list-disc space-y-1 pl-6 leading-7">
          <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
        </ul>
      );
    case "ordered_list":
      return (
        <ol className="my-3 list-decimal space-y-1 pl-6 leading-7">
          <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
        </ol>
      );
    case "list_item":
      return (
        <li>
          <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
        </li>
      );
    case "table":
      return (
        <div className="my-4 overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <tbody>
              <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
            </tbody>
          </table>
        </div>
      );
    case "table_row":
      return (
        <tr className="border-b border-border">
          <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
        </tr>
      );
    case "table_cell":
      return (
        <td className="border border-border px-3 py-2 align-top">
          <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
        </td>
      );
    case "code":
      return (
        <figure className="my-4 overflow-hidden rounded-lg border border-border bg-muted">
          {block.language ? (
            <figcaption className="border-b border-border px-4 py-1.5 font-mono text-xs text-muted-foreground">
              {block.language}
            </figcaption>
          ) : null}
          <pre className="overflow-x-auto p-4 text-sm leading-6">
            <code className="font-mono">{block.content}</code>
          </pre>
        </figure>
      );
    case "quote":
      return (
        <blockquote className="my-4 border-l-4 border-border pl-4 text-muted-foreground">
          <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
        </blockquote>
      );
    case "callout": {
      const { className, Icon, label } = CALLOUT_STYLES[block.kind];
      return (
        <aside
          role="note"
          aria-label={label}
          className={cn("my-4 flex gap-3 rounded-lg border p-4", className)}
        >
          <Icon className="mt-1 size-4 shrink-0" aria-hidden />
          <div className="min-w-0 flex-1">
            <BlockChildren blocks={block.children} citationNumbers={citationNumbers} />
          </div>
        </aside>
      );
    }
    case "component":
      return (
        <aside className="my-5 flex items-center gap-3 rounded-2xl border border-violet-200 bg-violet-50/70 p-4 text-violet-950">
          <Puzzle className="size-5 shrink-0 text-violet-600" aria-hidden />
          <div>
            <p className="text-sm font-semibold">知识组件</p>
            <p className="mt-0.5 text-xs text-violet-700">
              组件版本 {block.component_version} · 实体{" "}
              {block.entity_id.slice(0, 8)}
            </p>
          </div>
        </aside>
      );
    case "image":
      return (
        <figure className="my-6 overflow-hidden rounded-2xl border bg-card shadow-sm">
          <AssetImage
            revisionId={block.asset_revision_id}
            alt={block.alt_text}
          />
          {block.caption ? (
            <figcaption className="border-t px-4 py-3 text-center text-xs text-muted-foreground">
              {block.caption}
            </figcaption>
          ) : null}
        </figure>
      );
    case "video":
      return (
        <figure className="my-6 overflow-hidden rounded-2xl border bg-card shadow-sm">
          <AssetVideo
            revisionId={block.asset_revision_id}
            posterRevisionId={block.poster_asset_revision_id}
            title={block.title}
          />
          {block.caption ? (
            <figcaption className="border-t px-4 py-3 text-center text-xs text-muted-foreground">
              {block.caption}
            </figcaption>
          ) : null}
        </figure>
      );
    case "dataset_view":
      return (
        <Link
          href={`/datasets/views/${block.dataset_view_id}`}
          className="group my-5 flex items-center gap-3 rounded-2xl border border-cyan-200 bg-cyan-50/60 p-4 transition hover:border-cyan-300 hover:bg-cyan-50"
        >
          <Database className="size-5 text-cyan-700" aria-hidden />
          <span>
            <span className="block text-sm font-semibold text-cyan-950">
              可查询数据视图
            </span>
            <span className="mt-0.5 block text-xs text-cyan-700">
              打开筛选、排序与聚合视图
            </span>
          </span>
          <ExternalLink
            className="ml-auto size-4 text-cyan-700 transition-transform group-hover:translate-x-0.5"
            aria-hidden
          />
        </Link>
      );
    case "embed": {
      const href = safeHttpUrl(block.url);
      if (!href) {
        return (
          <span className="my-4 block rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
            无法打开不安全的嵌入地址
          </span>
        );
      }
      return (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="group my-5 flex items-center gap-3 rounded-2xl border bg-card p-4 transition hover:border-primary/25 hover:shadow-sm"
        >
          <ExternalLink className="size-5 text-primary" aria-hidden />
          <span className="min-w-0">
            <span className="block text-sm font-semibold">
              {block.title || "外部嵌入内容"}
            </span>
            <span className="mt-0.5 block truncate text-xs text-muted-foreground">
              {block.url}
            </span>
          </span>
        </a>
      );
    }
    case "divider":
      return <hr className="my-6 border-border" />;
  }
}

function BlockChildren({
  blocks,
  citationNumbers,
}: {
  blocks: Block[];
  citationNumbers: CitationNumbers;
}) {
  return (
    <>
      {blocks.map((child) => (
        <BlockView
          key={child.id}
          block={child}
          citationNumbers={citationNumbers}
        />
      ))}
    </>
  );
}

/** text node 的 marks 按顺序包裹为 strong/em/s/code。 */
function applyMarks(node: TextNode): ReactNode {
  let out: ReactNode = node.text;
  const marks: Mark[] = node.marks ?? [];
  for (const mark of marks) {
    switch (mark) {
      case "bold":
        out = <strong className="font-semibold">{out}</strong>;
        break;
      case "italic":
        out = <em>{out}</em>;
        break;
      case "strikethrough":
        out = <s>{out}</s>;
        break;
      case "code":
        out = (
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.875em]">
            {out}
          </code>
        );
        break;
    }
  }
  return out;
}

/** 渲染单个 inline 节点。 */
export function InlineNodeView({
  node,
  citationNumber = 1,
}: {
  node: InlineNode;
  citationNumber?: number;
}): ReactNode {
  switch (node.type) {
    case "text":
      return applyMarks(node);
    case "inline_code":
      return (
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.875em]">
          {node.text}
        </code>
      );
    case "page_reference":
      if ("resolution_status" in node) {
        // 未解析引用：不可点文本 + 样式标记，title 充当「页面不存在」提示。
        return (
          <span
            className="cursor-not-allowed text-muted-foreground underline decoration-dashed decoration-red-500/60 underline-offset-4"
            title={`页面不存在：${node.normalized_title}`}
            data-unresolved-ref={node.normalized_title}
          >
            {node.normalized_title}
          </span>
        );
      }
      return (
        <Link
          href={`/pages/${node.target_page_id}${
            node.target_heading_block_id
              ? `#${node.target_heading_block_id}`
              : ""
          }`}
          className="text-blue-600 underline-offset-4 hover:underline"
        >
          {node.display_text}
        </Link>
      );
    case "page_anchor_reference":
      return (
        <Link
          href={`/pages/${node.target_page_id}#${node.target_heading_block_id}`}
          className="text-blue-600 underline-offset-4 hover:underline"
        >
          {node.display_text}
        </Link>
      );
    case "external_link": {
      const href = safeHttpUrl(node.url);
      if (!href) {
        return <span data-unsafe-external-link="">{node.display_text}</span>;
      }
      return (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-baseline gap-0.5 text-blue-600 underline-offset-4 hover:underline"
        >
          {node.display_text}
          <ExternalLink className="size-3 self-center" aria-hidden />
        </a>
      );
    }
    case "entity_reference":
      return (
        <Link
          href={`/entities/${node.entity_id}`}
          data-entity-ref=""
          className="text-violet-600 underline-offset-4 hover:underline"
        >
          {node.display_text}
        </Link>
      );
    case "claim_reference":
      return (
        <Link
          href={`/claims/${node.claim_id}`}
          data-claim-ref={node.claim_id}
          className="rounded bg-emerald-500/10 px-1 text-emerald-700 underline-offset-4 hover:underline"
        >
          {node.display_text}
        </Link>
      );
    case "citation_reference":
      return (
        <sup
          data-citation-ref={node.citation_id}
          title={node.display_text ?? node.citation_id}
          className="ml-0.5 text-blue-600"
        >
          <Link href={`/citations/${node.citation_id}`} className="hover:underline">
            [{citationNumber}]
          </Link>
        </sup>
      );
    case "math":
      return (
        <code
          data-math-expression={node.expression}
          className="mx-0.5 rounded bg-sky-50 px-1.5 py-0.5 font-serif text-sky-950"
          title="数学表达式"
        >
          {node.expression}
        </code>
      );
    case "mention":
      return (
        <span
          data-actor-id={node.actor_id}
          className="mx-0.5 inline-flex items-center gap-0.5 rounded-full bg-violet-100 px-1.5 py-0.5 text-sm font-medium text-violet-700"
        >
          <AtSign className="size-3" aria-hidden />
          {node.display_text}
        </span>
      );
  }
}

function InlineNodes({
  nodes,
  citationNumbers,
}: {
  nodes: InlineNode[];
  citationNumbers: CitationNumbers;
}) {
  return (
    <>
      {nodes.map((node, i) => (
        <InlineNodeView
          key={i}
          node={node}
          citationNumber={
            node.type === "citation_reference"
              ? citationNumbers.get(node.citation_id)
              : undefined
          }
        />
      ))}
    </>
  );
}

/** Citation 按 citation_id 首次出现顺序编号；同一 Citation 重复引用复用序号。 */
function collectCitationNumbers(document: Document): CitationNumbers {
  const numbers = new Map<string, number>();
  const visit = (blocks: Block[]) => {
    for (const block of blocks) {
      if (block.type === "heading" || block.type === "paragraph") {
        for (const node of block.content) {
          if (
            node.type === "citation_reference" &&
            !numbers.has(node.citation_id)
          ) {
            numbers.set(node.citation_id, numbers.size + 1);
          }
        }
      } else if ("children" in block) {
        visit(block.children as Block[]);
      }
    }
  };
  visit(document.children);
  return numbers;
}

function citationNumbersFromOrder(order: readonly string[]): CitationNumbers {
  return new Map(order.map((citationId, index) => [citationId, index + 1]));
}

/** 渲染整份 AST 文档。调用方负责先经 parseDocument 校验。 */
export function AstDocument({
  document,
  citationOrder,
}: {
  document: Document;
  citationOrder?: readonly string[];
}) {
  const citationNumbers = citationOrder
    ? citationNumbersFromOrder(citationOrder)
    : collectCitationNumbers(document);
  return (
    <div data-ast-document>
      <BlockChildren
        blocks={document.children}
        citationNumbers={citationNumbers}
      />
    </div>
  );
}
