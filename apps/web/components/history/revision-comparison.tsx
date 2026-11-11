"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, ArrowUp, ArrowRightLeft } from "lucide-react";
import type { DocumentDiff, RevisionDetail } from "../../../../contracts/generated/typescript";
import { Button } from "@/components/ui/button";
import { useSession } from "@/lib/auth";
import { historyApi } from "@/lib/api";
import useSWR from "swr";
import { parseDocument } from "@/lib/ast/schema";
import { blockElements, decorateComparison, filterComparison } from "@/lib/history/decorate-diff";
import { buildVisualChanges, changeLabels, indexDocument } from "@/lib/history/visual-diff";
import { cn } from "@/lib/utils";
import { DiffView } from "./diff-view";
import { formatDateTime, revisionAuthorLabel } from "./utils";
import styles from "./revision-comparison.module.css";

export function RevisionComparison({ pageId, before, after, diff }: { pageId: string; before: RevisionDetail; after: RevisionDetail; diff: DocumentDiff }) {
  const { session } = useSession();
  const [mode, setMode] = useState("page");
  const [onlyChanges, setOnlyChanges] = useState(false);
  const [linked, setLinked] = useState(true);
  const [mobileSide, setMobileSide] = useState(1);
  const [selected, setSelected] = useState(0);
  const roots = useRef<(HTMLDivElement | null)[]>([]);
  const panes = useRef<(HTMLDivElement | null)[]>([]);
  const scrollLock = useRef(false);
  const scrollFrame = useRef(0);
  const oldPreview = useSWR(["history:preview", session?.actorId, session?.wikiId, pageId, before.revision.id], () => historyApi().getRevision({ id: pageId, rid: before.revision.id, includeRender: true }), { fallbackData: before, revalidateOnMount: false, revalidateOnFocus: false });
  const newPreview = useSWR(["history:preview", session?.actorId, session?.wikiId, pageId, after.revision.id], () => historyApi().getRevision({ id: pageId, rid: after.revision.id, includeRender: true }), { fallbackData: after, revalidateOnMount: false, revalidateOnFocus: false });
  const details = [oldPreview.data ?? before, newPreview.data ?? after];
  const oldHTML = details[0].html ?? "", newHTML = details[1].html ?? "";
  const indexes = useMemo(() => [indexDocument(parseDocument(before.astJson)), indexDocument(parseDocument(after.astJson))] as const, [before.astJson, after.astJson]);
  const changes = useMemo(() => buildVisualChanges(indexes[0], indexes[1], diff), [indexes, diff]);
  const current = changes[selected];
  const hasDynamic = indexes.some(index => [...index.values()].some(({ block }) => block.type === "component" || ("content" in block && Array.isArray(block.content) && block.content.some(node => node.type === "claim_reference"))));

  useEffect(() => {
    const [oldRoot, newRoot] = roots.current;
    if (!oldRoot || !newRoot) return;
    decorateComparison([oldRoot, newRoot], [oldHTML, newHTML], changes, [indexes[0], indexes[1]]);
  }, [oldHTML, newHTML, changes, indexes, mode]);

  useEffect(() => {
    roots.current.forEach((root, side) => {
      if (!root) return;
      filterComparison(root, indexes[side], changes, onlyChanges);
      for (const [id, el] of blockElements(root)) el.toggleAttribute("data-history-selected", id === current?.id);
    });
  }, [onlyChanges, current, indexes, changes, oldHTML, newHTML, mode]);

  useEffect(() => () => cancelAnimationFrame(scrollFrame.current), []);

  const navigate = (index: number) => {
    if (!changes.length) return;
    const next = Math.max(0, Math.min(changes.length - 1, index));
    setSelected(next);
    scrollLock.current = true;
    roots.current.forEach((root, side) => {
      const pane = panes.current[side];
      if (!root || !pane) return;
      const blocks = blockElements(root);
      const entry = indexes[side].get(changes[next].id);
      for (const id of entry?.ancestors ?? []) {
        const ancestor = blocks.get(id);
        if (ancestor instanceof HTMLDetailsElement) ancestor.open = true;
      }
      const target = blocks.get(changes[next].id);
      if (target) pane.scrollTop += target.getBoundingClientRect().top - pane.getBoundingClientRect().top - 24;
    });
    cancelAnimationFrame(scrollFrame.current);
    scrollFrame.current = requestAnimationFrame(() => { scrollLock.current = false; });
  };
  const syncScroll = (side: number) => {
    if (!linked || scrollLock.current || window.innerWidth < 768) return;
    const source = panes.current[side], target = panes.current[1 - side], root = roots.current[side], other = roots.current[1 - side];
    if (!source || !target || !root || !other) return;
    const targetBlocks = blockElements(other), top = source.getBoundingClientRect().top;
    const moved = new Set(changes.filter(change => change.types.includes("moved")).map(change => change.id));
    const candidates = [...blockElements(root)].filter(([id, el]) => targetBlocks.has(id) && !moved.has(id) && !indexes[side].get(id)?.ancestors.some(parent => moved.has(parent)) && el.getClientRects().length);
    const anchor = candidates.find(([, el]) => el.getBoundingClientRect().bottom > top + 16);
    if (!anchor) return;
    const paired = targetBlocks.get(anchor[0])!;
    if (!paired.getClientRects().length) return;
    scrollLock.current = true;
    target.scrollTop += paired.getBoundingClientRect().top - target.getBoundingClientRect().top - (anchor[1].getBoundingClientRect().top - top);
    cancelAnimationFrame(scrollFrame.current);
    scrollFrame.current = requestAnimationFrame(() => { scrollLock.current = false; });
  };

  return <section className={styles.comparison} aria-label="版本对比">
    <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-4">
      <div className="flex gap-1" role="group" aria-label="对比方式">
        <Button size="sm" variant={mode === "page" ? "default" : "outline"} aria-pressed={mode === "page"} onClick={() => setMode("page")}>页面对比</Button>
        <Button size="sm" variant={mode === "structure" ? "default" : "outline"} aria-pressed={mode === "structure"} onClick={() => setMode("structure")}>结构详情 · JSON</Button>
      </div>
      <Button variant="outline" size="sm" asChild><Link href={`/pages/${pageId}/diff?from=${after.revision.id}&to=${before.revision.id}`}><ArrowRightLeft className="size-4" />交换版本</Link></Button>
    </div>
    {mode === "structure" ? <div className="py-5"><DiffView diff={diff} /></div> : <>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-3 py-4 text-sm">
        <div className="flex flex-wrap gap-3">{Object.entries(changeLabels).map(([type, label]) => <span key={type} data-change-count={type} className={styles[type]}>{label} {changes.filter(c => c.types.includes(type as keyof typeof changeLabels)).length}</span>)}</div>
        <label className="flex cursor-pointer items-center gap-2"><input type="checkbox" checked={onlyChanges} onChange={e => setOnlyChanges(e.target.checked)} />仅看变更</label>
        <label className="hidden cursor-pointer items-center gap-2 md:flex"><input type="checkbox" checked={linked} onChange={e => setLinked(e.target.checked)} />联动滚动</label>
        <div className="ml-auto flex items-center gap-2">
          <Button aria-label="上一处变更" size="icon-sm" variant="outline" disabled={!changes.length || selected === 0} onClick={() => navigate(selected - 1)}><ArrowUp className="size-4" /></Button>
          <span aria-live="polite" className="min-w-10 text-center tabular-nums">{changes.length ? selected + 1 : 0} / {changes.length}</span>
          <Button aria-label="下一处变更" size="icon-sm" variant="outline" disabled={!changes.length || selected === changes.length - 1} onClick={() => navigate(selected + 1)}><ArrowDown className="size-4" /></Button>
        </div>
      </div>
      {hasDynamic && <p className="mb-4 text-xs text-muted-foreground">引用数据按当前状态展示；对比的是各版本保存的文字、布局和组件配置。</p>}
      {!changes.length && <p className="mb-4 rounded-lg border border-dashed p-4 text-sm text-muted-foreground">两版相同，没有内容或块顺序变化。</p>}
      <div className={styles.layout}>
        {changes.length > 0 && <nav className={styles.navigation} aria-label="变更位置">{changes.map((change, index) => <button key={change.id} type="button" className={cn(styles.change, index === selected && styles.active)} aria-current={index === selected ? "true" : undefined} onClick={() => navigate(index)}>
          <span className="block text-sm font-medium">{change.label} · {change.types.map(type => changeLabels[type]).join("、")}</span>
          <span className="mt-1 line-clamp-2 block text-xs text-muted-foreground">{change.excerpt || "布局或配置变更"}</span>
        </button>)}</nav>}
        <div className="min-w-0 flex-1">
          <div className="mb-3 flex gap-2 md:hidden" role="group" aria-label="查看版本">{["修改前", "修改后"].map((label, side) => <Button key={label} size="sm" variant={mobileSide === side ? "default" : "outline"} aria-pressed={mobileSide === side} onClick={() => { setMobileSide(side); requestAnimationFrame(() => navigate(selected)); }}>{label}</Button>)}</div>
          <div className={styles.documents}>{details.map((detail, side) => <article key={side} className={cn(styles.document, mobileSide !== side && styles.mobileHidden)} data-history-side={side === 0 ? "before" : "after"}>
            <header className="border-b px-4 py-3 text-xs">
              <div className="flex flex-wrap justify-between gap-1"><strong>{side === 0 ? "修改前" : "修改后"}</strong><Link className="text-muted-foreground hover:underline" href={`/pages/${pageId}/history/${detail.revision.id}`}>{formatDateTime(detail.revision.createdAt)}</Link></div>
              <p className="mt-1 text-muted-foreground">{revisionAuthorLabel(detail.revision)}{detail.revision.summary ? ` · ${detail.revision.summary}` : ""}</p>
            </header>
            {current && !indexes[side].has(current.id) && <p className="border-b bg-muted/50 px-4 py-2 text-xs text-muted-foreground" role="status">{side === 0 ? "此处内容在修改后新增" : "此处内容已在修改后删除"}</p>}
            <div className={styles.pane} ref={el => { panes.current[side] = el; }} onScroll={() => syncScroll(side)} tabIndex={0} aria-label={side === 0 ? "修改前页面" : "修改后页面"}>
              {detail.html === undefined && <div className="p-4 text-sm text-muted-foreground"><p>页面预览暂时不可用，仍可查看结构详情。</p><Button variant="outline" size="sm" className="mt-2" disabled={side === 0 ? oldPreview.isValidating : newPreview.isValidating} onClick={() => { void (side === 0 ? oldPreview : newPreview).mutate().catch(() => undefined); }}>重试预览</Button></div>}
              <div ref={el => { roots.current[side] = el; }} className="wiki-rendered" data-history-document="" onClickCapture={event => { if ((event.target as Element).closest("a,button")) { event.preventDefault(); event.stopPropagation(); } }} />
            </div>
          </article>)}</div>
        </div>
      </div>
    </>}
  </section>;
}
