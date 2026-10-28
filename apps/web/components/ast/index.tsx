// Typed Block AST v1 → React 的原生渲染器（编辑预览、历史与投影降级路径）。
// 当前阅读页优先消费版本化 RenderedPage HTML，确保 Component/Claim 动态值与
// Worker 投影保持一致；AST 渲染器仍是不可变快照的客户端安全兜底。
import Link from "next/link";
import {
  AtSign,
  Check,
  ChevronRight,
  ExternalLink,
  Info,
  OctagonAlert,
  Puzzle,
  TriangleAlert,
} from "lucide-react";
import type { CSSProperties, ReactNode } from "react";

import { cn } from "@/lib/utils";
import type {
  Block,
  CalloutColor,
  CalloutKind,
  Document,
  InlineNode,
  Mark,
  TextNode,
} from "@/lib/ast/schema";
import { safeHttpUrl } from "@/lib/http-url";
import { compactId } from "@/lib/display-id";
import {
  AssetAudio,
  AssetFile,
  AssetImage,
  AssetPDF,
  AssetVideo,
} from "@/components/ast/asset-media";
import { interactiveEmbedUrl } from "@/lib/embed";

const HEADING_STYLES: Record<number, string> = {
  1: "mt-8 mb-4 text-3xl font-bold tracking-tight",
  2: "mt-8 mb-3 text-2xl font-semibold tracking-tight",
  3: "mt-6 mb-3 text-xl font-semibold",
  4: "mt-6 mb-2 text-lg font-semibold",
  5: "mt-4 mb-2 text-base font-semibold",
  6: "mt-4 mb-2 text-sm font-semibold text-muted-foreground",
};

function isolatedHTMLDocument(content: string): string {
  const policy =
    "default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'; " +
    "form-action 'none'; base-uri 'none'";
  return (
    '<!doctype html><html><head><meta charset="utf-8">' +
    `<meta http-equiv="Content-Security-Policy" content="${policy}">` +
    '<meta name="viewport" content="width=device-width,initial-scale=1">' +
    "<style>html{color-scheme:light}body{margin:0;padding:12px;font:14px/1.5 system-ui,sans-serif;" +
    "overflow-wrap:anywhere}img,video,canvas,svg{max-width:100%;height:auto}" +
    "table{max-width:100%;border-collapse:collapse}pre{max-width:100%;overflow:auto}</style>" +
    `</head><body>${content}</body></html>`
  );
}

function formatDuration(seconds: number | undefined): string | null {
  if (seconds === undefined) return null;
  const total = Math.max(0, Math.round(seconds));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
}

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
  const content = (
    <InlineNodes
      nodes={block.content}
      citationNumbers={citationNumbers}
      blockId={block.id}
    />
  );
  if (block.is_toggleable) {
    return (
      <details className="group my-4">
        <summary
          id={block.id}
          className={cn(
            "flex cursor-pointer list-none items-center gap-2 scroll-mt-20 [&::-webkit-details-marker]:hidden",
            HEADING_STYLES[level],
          )}
        >
          <ChevronRight
            className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-90"
            aria-hidden
          />
          <span role="heading" aria-level={level}>
            {content}
          </span>
        </summary>
        <div className="ml-6 border-l border-border pl-4">
          <BlockChildren
            blocks={block.children ?? []}
            citationNumbers={citationNumbers}
          />
        </div>
      </details>
    );
  }
  return (
    <Tag
      id={block.id}
      className={cn(
        "scroll-mt-20 border-b border-transparent",
        HEADING_STYLES[level],
      )}
    >
      {content}
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

const CALLOUT_COLOR_STYLES: Record<CalloutColor, string> = {
  default: "border-border bg-muted/40 text-foreground",
  gray: "border-zinc-500/40 bg-zinc-500/10 text-foreground",
  brown: "border-amber-900/40 bg-amber-900/10 text-foreground",
  orange: "border-orange-500/40 bg-orange-500/10 text-foreground",
  yellow: "border-yellow-500/40 bg-yellow-500/10 text-foreground",
  green: "border-green-500/40 bg-green-500/10 text-foreground",
  blue: "border-blue-500/40 bg-blue-500/10 text-foreground",
  purple: "border-purple-500/40 bg-purple-500/10 text-foreground",
  pink: "border-pink-500/40 bg-pink-500/10 text-foreground",
  red: "border-red-500/40 bg-red-500/10 text-foreground",
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
          <InlineNodes
            nodes={block.content}
            citationNumbers={citationNumbers}
            blockId={block.id}
          />
        </p>
      );
    case "todo":
      return (
        <div className="my-3">
          <div className="flex items-start gap-3 leading-7">
            <span
              className={cn(
                "mt-1 flex size-5 shrink-0 items-center justify-center rounded border",
                block.checked
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-background",
              )}
              role="checkbox"
              aria-checked={block.checked}
            >
              {block.checked ? <Check className="size-3.5" aria-hidden /> : null}
            </span>
            <span className={cn(block.checked && "text-muted-foreground line-through")}>
              <InlineNodes
                nodes={block.content}
                citationNumbers={citationNumbers}
                blockId={block.id}
              />
            </span>
          </div>
          {block.children?.length ? (
            <div className="ml-8 border-l border-border pl-4">
              <BlockChildren
                blocks={block.children}
                citationNumbers={citationNumbers}
              />
            </div>
          ) : null}
        </div>
      );
    case "toggle":
      return (
        <details className="group my-3 rounded-lg border border-border">
          <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 [&::-webkit-details-marker]:hidden">
            <ChevronRight
              className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-90"
              aria-hidden
            />
            <InlineNodes
              nodes={block.content}
              citationNumbers={citationNumbers}
              blockId={block.id}
            />
          </summary>
          <div className="border-t border-border px-4 pb-3 pl-10">
            <BlockChildren
              blocks={block.children ?? []}
              citationNumbers={citationNumbers}
            />
          </div>
        </details>
      );
    case "breadcrumb":
      return (
        <nav className="wiki-breadcrumb my-4" aria-label="Breadcrumb">
          <ol className="flex min-w-0 flex-wrap items-center gap-1.5 text-sm text-muted-foreground">
            {block.items.map((item, index) => (
              <li
                key={`${item.target_page_id ?? "current"}-${index}`}
                className="contents"
              >
                {item.target_page_id ? (
                  <Link
                    href={`/pages/${item.target_page_id}`}
                    className="min-w-0 truncate hover:text-foreground hover:underline"
                  >
                    {item.display_text}
                  </Link>
                ) : (
                  <span
                    className="min-w-0 truncate text-foreground"
                    aria-current="page"
                  >
                    {item.display_text}
                  </span>
                )}
                {index < block.items.length - 1 ? (
                  <ChevronRight className="size-3.5 shrink-0" aria-hidden />
                ) : null}
              </li>
            ))}
          </ol>
        </nav>
      );
    case "column_list":
      return (
        <div className="wiki-column-list my-4">
          <BlockChildren
            blocks={block.children}
            citationNumbers={citationNumbers}
          />
        </div>
      );
    case "column":
      return (
        <section
          className="wiki-column min-w-0"
          style={{ "--column-ratio": block.width_ratio } as CSSProperties}
        >
          <BlockChildren
            blocks={block.children}
            citationNumbers={citationNumbers}
          />
        </section>
      );
    case "button":
      return (
        <button
          type="button"
          className="wiki-button my-3"
          data-page-button=""
          data-block-id={block.id}
          data-actions-json={JSON.stringify(block.actions)}
          data-confirmation={block.confirmation}
        >
          {block.label}
        </button>
      );
    case "html":
      return (
        <figure className="html-block my-4">
          <iframe
            sandbox=""
            loading="lazy"
            referrerPolicy="no-referrer"
            title={block.caption || "HTML content"}
            height={block.height}
            srcDoc={isolatedHTMLDocument(block.html)}
          />
          {block.caption ? <figcaption>{block.caption}</figcaption> : null}
        </figure>
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
        <figure className="my-4 overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            {block.caption ? (
              <caption className="caption-top px-3 py-2 text-left text-sm text-muted-foreground">
                {block.caption}
              </caption>
            ) : null}
            {block.has_column_header && block.children.length > 0 ? (
              <thead>
                <TableRowView
                  row={block.children[0]}
                  rowIndex={0}
                  columnHeader
                  rowHeader={Boolean(block.has_row_header)}
                  citationNumbers={citationNumbers}
                />
              </thead>
            ) : null}
            <tbody>
              {block.children
                .slice(block.has_column_header ? 1 : 0)
                .map((row, index) => (
                  <TableRowView
                    key={row.id}
                    row={row}
                    rowIndex={index + (block.has_column_header ? 1 : 0)}
                    columnHeader={false}
                    rowHeader={Boolean(block.has_row_header)}
                    citationNumbers={citationNumbers}
                  />
                ))}
            </tbody>
          </table>
        </figure>
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
          {block.caption || block.language ? (
            <figcaption className="flex items-center justify-between gap-3 border-b border-border px-4 py-2 text-xs text-muted-foreground">
              <span>{block.caption}</span>
              {block.language ? <span className="font-mono">{block.language}</span> : null}
            </figcaption>
          ) : null}
          <pre className="overflow-x-auto p-4 text-sm leading-6">
            <code className="font-mono">{block.content}</code>
          </pre>
        </figure>
      );
    case "preformatted":
      return (
        <figure
          className="my-4 overflow-hidden rounded-lg border border-border bg-muted"
          data-preformatted-kind={block.kind}
        >
          {block.caption ? (
            <figcaption className="border-b border-border px-4 py-2 text-xs text-muted-foreground">
              {block.caption}
            </figcaption>
          ) : null}
          <pre className="overflow-x-auto p-4 text-sm leading-6">
            <code className="font-mono">{block.content}</code>
          </pre>
        </figure>
      );
    case "equation":
      return (
        <figure
          className="my-4 overflow-x-auto rounded-lg border border-border bg-muted/40 px-4 py-5 text-center"
          data-math-expression={block.expression}
        >
          <div className="font-mono text-sm">{block.expression}</div>
          {block.caption ? (
            <figcaption className="mt-3 text-xs text-muted-foreground">
              {block.caption}
            </figcaption>
          ) : null}
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
      const colorClassName = block.color
        ? CALLOUT_COLOR_STYLES[block.color]
        : className;
      return (
        <aside
          role="note"
          aria-label={label}
          className={cn("my-4 flex gap-3 rounded-lg border p-4", colorClassName)}
        >
          {block.icon ? (
            <span className="mt-0.5 shrink-0 text-base" aria-hidden>
              {block.icon}
            </span>
          ) : (
            <Icon className="mt-1 size-4 shrink-0" aria-hidden />
          )}
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
              {compactId(block.entity_id)}
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
    case "audio":
      return (
        <figure className="wiki-audio my-5 overflow-hidden rounded-lg border bg-card">
          <div className="border-b px-4 py-3">
            <p className="text-sm font-semibold">{block.title}</p>
            {block.artist || block.album ? (
              <p className="mt-1 text-xs text-muted-foreground">
                {[block.artist, block.album, formatDuration(block.duration_seconds)]
                  .filter(Boolean)
                  .join(" · ")}
              </p>
            ) : block.duration_seconds !== undefined ? (
              <p className="mt-1 text-xs text-muted-foreground">
                {formatDuration(block.duration_seconds)}
              </p>
            ) : null}
          </div>
          <div className="p-4">
            <AssetAudio
              revisionId={block.asset_revision_id}
              title={block.title}
            />
          </div>
          {block.caption ? <figcaption>{block.caption}</figcaption> : null}
        </figure>
      );
    case "file":
      return (
        <aside className="wiki-file my-5">
          <AssetFile
            revisionId={block.asset_revision_id}
            name={block.name}
          />
          {block.caption ? (
            <p className="mt-2 text-xs text-muted-foreground">{block.caption}</p>
          ) : null}
        </aside>
      );
    case "pdf":
      return (
        <figure className="wiki-pdf my-6 overflow-hidden rounded-lg border bg-card">
          <AssetPDF
            revisionId={block.asset_revision_id}
            title={block.title || "PDF document"}
            startPage={block.start_page}
            height={block.height}
          />
          {block.caption ? <figcaption>{block.caption}</figcaption> : null}
        </figure>
      );
    case "bookmark":
    case "link_preview": {
      const href = safeHttpUrl(block.url);
      if (!href) {
        return (
          <span className="my-4 block rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            无法打开不安全的预览地址
          </span>
        );
      }
      const siteName = block.site_name || new URL(href).hostname;
      return (
        <figure
          className={cn(
            "my-5 overflow-hidden rounded-lg border bg-card transition hover:border-primary/25 hover:shadow-sm",
            block.type === "bookmark"
              ? "wiki-bookmark"
              : "wiki-link-preview border-l-4 border-l-primary/50",
          )}
        >
          <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className="group block p-4"
          >
            <span className="text-[11px] font-medium text-muted-foreground">
              {siteName}
            </span>
            <strong className="mt-1 block text-sm">{block.title}</strong>
            {block.description ? (
              <span className="mt-1.5 line-clamp-2 block text-xs leading-5 text-muted-foreground">
                {block.description}
              </span>
            ) : null}
            <ExternalLink
              className="mt-3 size-3.5 text-primary transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </a>
          {block.type === "bookmark" && block.caption ? (
            <figcaption>{block.caption}</figcaption>
          ) : null}
        </figure>
      );
    }
    case "dataset_view":
      return (
        <div
          className="my-5 rounded-lg border border-dashed bg-muted/40 p-4 text-sm text-muted-foreground"
          data-retired-block="dataset_view"
        >
          此旧结构化视图已停用
        </div>
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
      const embedUrl = interactiveEmbedUrl(block.provider, href);
      if (embedUrl) {
        return (
          <figure
            className="interactive-embed my-6 overflow-hidden rounded-lg border bg-black"
            data-embed-provider={block.provider}
          >
            <iframe
              src={embedUrl}
              title={block.title || "Interactive embed"}
              loading="lazy"
              referrerPolicy="strict-origin-when-cross-origin"
              sandbox="allow-scripts allow-same-origin allow-presentation"
              allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; fullscreen"
              allowFullScreen
              className="block aspect-video w-full border-0"
            />
            {block.caption ? <figcaption>{block.caption}</figcaption> : null}
          </figure>
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

function TableRowView({
  row,
  rowIndex,
  columnHeader,
  rowHeader,
  citationNumbers,
}: {
  row: Extract<Block, { type: "table_row" }>;
  rowIndex: number;
  columnHeader: boolean;
  rowHeader: boolean;
  citationNumbers: CitationNumbers;
}) {
  return (
    <tr className="border-b border-border">
      {row.children.map((cell, cellIndex) => {
        const header = columnHeader || (rowHeader && cellIndex === 0);
        const Cell = header ? "th" : "td";
        return (
          <Cell
            key={cell.id}
            scope={header ? (columnHeader ? "col" : "row") : undefined}
            data-row={rowIndex}
            data-column={cellIndex}
            className="border border-border px-3 py-2 text-left align-top"
          >
            <BlockChildren blocks={cell.children} citationNumbers={citationNumbers} />
          </Cell>
        );
      })}
    </tr>
  );
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
      case "underline":
        out = <u>{out}</u>;
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
  if (node.text_color || node.background_color) {
    out = (
      <span
        data-text-color={node.text_color}
        data-background-color={node.background_color}
      >
        {out}
      </span>
    );
  }
  return out;
}

/** 渲染单个 inline 节点。 */
export function InlineNodeView({
  node,
  citationNumber = 1,
  citationAnchorId,
}: {
  node: InlineNode;
  citationNumber?: number;
  citationAnchorId?: string;
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
        <a
          href={`/concepts/${node.entity_id}`}
          data-entity-ref={node.entity_id}
          className="text-violet-600 underline-offset-4 hover:underline"
        >
          {node.display_text}
        </a>
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
          id={citationAnchorId}
          data-citation-ref={node.citation_id}
          title={node.display_text ?? node.citation_id}
          className="ml-0.5 scroll-mt-20 text-blue-600"
        >
          <a
            href={`#cite-note-${citationNumber}`}
            aria-label={`参考文献 ${citationNumber}`}
            className="hover:underline"
          >
            [{citationNumber}]
          </a>
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
    case "date_mention":
      return (
        <time
          dateTime={node.date}
          data-date-mention=""
          data-end-date={node.end_date}
          className="mx-0.5 rounded bg-amber-100 px-1.5 py-0.5 text-sm font-medium text-amber-900"
        >
          {node.display_text}
        </time>
      );
    case "database_mention":
      return (
        <span
          data-database-mention=""
          className="mx-0.5 rounded bg-muted px-1.5 py-0.5 text-sm font-medium text-muted-foreground"
        >
          {node.display_text}
        </span>
      );
  }
}

function InlineNodes({
  nodes,
  citationNumbers,
  blockId,
}: {
  nodes: InlineNode[];
  citationNumbers: CitationNumbers;
  blockId: string;
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
          citationAnchorId={
            node.type === "citation_reference"
              ? `cite-ref-${blockId}-${i}`
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
      if (
        block.type === "heading" ||
        block.type === "paragraph" ||
        block.type === "todo" ||
        block.type === "toggle"
      ) {
        for (const node of block.content) {
          if (
            node.type === "citation_reference" &&
            !numbers.has(node.citation_id)
          ) {
            numbers.set(node.citation_id, numbers.size + 1);
          }
        }
      }
      if ("children" in block) {
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
