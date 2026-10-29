"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { projectionApi } from "@/lib/api";

export function StableAnchorResolver({ pageId }: { pageId: string }) {
  const router = useRouter();

  useEffect(() => {
    let generation = 0;

    const resolveHash = async () => {
      const currentGeneration = ++generation;
      const rawHash = window.location.hash.slice(1);
      if (!rawHash) return;

      let slug: string;
      try {
        slug = decodeURIComponent(rawHash);
      } catch {
        return;
      }
      const existing = document.getElementById(slug);
      if (existing) {
        for (
          let ancestor = existing.parentElement;
          ancestor;
          ancestor = ancestor.parentElement
        ) {
          if (ancestor instanceof HTMLDetailsElement) ancestor.open = true;
        }
        existing.scrollIntoView();
        return;
      }

      try {
        const target = await projectionApi().resolvePageAnchor({
          id: pageId,
          slug,
        });
        if (currentGeneration !== generation) return;

        if (target.pageId !== pageId) {
          router.push(
            `/pages/${encodeURIComponent(target.pageId)}#${encodeURIComponent(target.blockId)}`,
          );
          return;
        }
        const element = document.getElementById(target.blockId);
        if (!element) return;
        window.history.replaceState(
          null,
          "",
          `${window.location.pathname}${window.location.search}#${target.blockId}`,
        );
        element.scrollIntoView();
      } catch {
        // Unknown hashes keep the browser's normal no-op behavior.
      }
    };

    const handleClick = (event: MouseEvent) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) return;
      const link = event.target instanceof Element
        ? event.target.closest("a[href]")
        : null;
      if (
        !(link instanceof HTMLAnchorElement) ||
        link.hasAttribute("download") ||
        (link.target && link.target !== "_self")
      ) return;
      if (
        link.origin === window.location.origin &&
        link.pathname === window.location.pathname &&
        link.search === window.location.search &&
        link.hash &&
        link.hash === window.location.hash
      ) void resolveHash();
    };

    void resolveHash();
    window.addEventListener("hashchange", resolveHash);
    document.addEventListener("click", handleClick);
    return () => {
      generation += 1;
      window.removeEventListener("hashchange", resolveHash);
      document.removeEventListener("click", handleClick);
    };
  }, [pageId, router]);

  return null;
}
