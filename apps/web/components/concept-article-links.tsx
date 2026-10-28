"use client";

import { ArrowUpRight, BookOpen, LoaderCircle, RefreshCw, X } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Popover } from "radix-ui";
import {
  useEffect,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
} from "react";
import useSWR from "swr";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { knowledgeApi } from "@/lib/api";

const entityIDSchema = z.uuid();

/**
 * One delegated handler covers both trusted server HTML and the AST fallback,
 * including sections inserted later. SWR only resolves the clicked concept.
 */
export function ConceptArticleLinks({
  wikiId,
  language,
  children,
}: {
  wikiId: string;
  language: string;
  children: ReactNode;
}) {
  const router = useRouter();
  const anchorRef = useRef<HTMLAnchorElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState<{ id: string; label: string } | null>(null);
  const resolution = useSWR(
    active ? ["concept:articles", wikiId, active.id, language] : null,
    ([, , id, preferredLanguage]) =>
      knowledgeApi().resolveEntityArticles({ id, language: preferredLanguage }),
    { revalidateOnFocus: false, revalidateOnMount: true, shouldRetryOnError: false },
  );
  const items = resolution.data?.items;

  useEffect(() => {
    if (!items || resolution.isValidating || resolution.error) return;
    if (items.length === 1) {
      const frame = requestAnimationFrame(() => {
        setActive(null);
        router.push(`/pages/${items[0].pageId}`);
      });
      return () => cancelAnimationFrame(frame);
    } else {
      contentRef.current
        ?.querySelector<HTMLAnchorElement>("[data-article-choice]")
        ?.focus();
    }
  }, [items, resolution.error, resolution.isValidating, router]);

  function onConceptClick(event: MouseEvent<HTMLDivElement>) {
    if (
      event.defaultPrevented || event.button !== 0 ||
      event.metaKey || event.ctrlKey || event.shiftKey || event.altKey
    ) return;
    const target = event.target;
    if (!(target instanceof Element)) return;
    const anchor = target.closest<HTMLAnchorElement>("a[data-entity-ref]");
    if (!anchor || anchor.target === "_blank" || anchor.hasAttribute("download")) return;
    // The href fallback also supports projections built before data-entity-ref
    // carried its stable ID.
    const parsed = entityIDSchema.safeParse(
      anchor.dataset.entityRef || anchor.pathname.split("/").at(-1),
    );
    if (!parsed.success) return;
    event.preventDefault();
    anchorRef.current = anchor;
    setActive({ id: parsed.data, label: anchor.textContent?.trim() || "这个概念" });
  }

  return (
    <div onClickCapture={onConceptClick}>
      {children}
      <Popover.Root
        open={active !== null}
        onOpenChange={(open) => {
          if (!open) setActive(null);
        }}
      >
        <Popover.Anchor virtualRef={anchorRef} />
        <Popover.Portal>
          <Popover.Content
            ref={contentRef}
            side="bottom"
            align="start"
            sideOffset={8}
            collisionPadding={16}
            aria-label={`「${active?.label ?? ""}」的条目`}
            className="z-50 flex max-h-[min(28rem,var(--radix-popover-content-available-height))] w-80 max-w-[calc(100vw-2rem)] flex-col overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg outline-none"
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              anchorRef.current?.focus();
            }}
            onInteractOutside={(event) => {
              if (
                event.target instanceof Element &&
                event.target.closest("a[data-entity-ref]")
              ) event.preventDefault();
            }}
          >
            <div className="flex items-start justify-between gap-3 border-b px-4 py-3">
              <div className="min-w-0">
                <p className="break-words text-sm font-semibold">{active?.label}</p>
                <p className="mt-1 text-xs text-muted-foreground">选择要阅读的条目</p>
              </div>
              <Popover.Close asChild>
                <Button variant="ghost" size="icon-sm" aria-label="关闭条目选择">
                  <X aria-hidden className="size-4" />
                </Button>
              </Popover.Close>
            </div>
            {resolution.isLoading || (items?.length === 1 && !resolution.error) ? (
              <p role="status" className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
                <LoaderCircle aria-hidden className="size-4 animate-spin" />
                {items?.length === 1 ? "正在打开条目…" : "正在查找条目…"}
              </p>
            ) : resolution.error ? (
              <div className="space-y-3 p-4">
                <p role="alert" className="text-sm text-muted-foreground">条目解析失败，请重试。</p>
                <Button variant="outline" size="sm" onClick={() => void resolution.mutate()}>
                  <RefreshCw aria-hidden />
                  重试
                </Button>
              </div>
            ) : items?.length ? (
              <nav aria-label="概念条目" className="min-h-0 overflow-y-auto p-2">
                {items.map((item) => (
                  <Link
                    key={item.pageId}
                    data-article-choice=""
                    href={`/pages/${item.pageId}`}
                    onClick={() => setActive(null)}
                    onKeyDown={(event) => {
                      if (
                        event.key === "Enter" &&
                        !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey
                      ) {
                        event.preventDefault();
                        setActive(null);
                        router.push(`/pages/${item.pageId}`);
                      }
                    }}
                    className="flex items-start gap-2.5 rounded-lg px-3 py-2.5 hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                  >
                    <BookOpen aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                    <span className="min-w-0">
                      <span className="block break-words text-sm font-medium">{item.displayTitle}</span>
                      <span className="mt-1 block text-xs text-muted-foreground">{item.namespace} · {item.language}</span>
                    </span>
                  </Link>
                ))}
              </nav>
            ) : (
              <div className="space-y-3 p-4">
                <p className="text-sm text-muted-foreground">暂无可阅读条目。</p>
                <Link
                  href={`/entities/${resolution.data?.entityId ?? active?.id}`}
                  className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
                >
                  查看概念信息
                  <ArrowUpRight aria-hidden className="size-3.5" />
                </Link>
              </div>
            )}
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
    </div>
  );
}
