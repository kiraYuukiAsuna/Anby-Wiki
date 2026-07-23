"use client";

import { useEffect } from "react";

import { projectionApi } from "@/lib/api";

export function StableAnchorResolver({ pageId }: { pageId: string }) {
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
          window.location.assign(`/pages/${target.pageId}#${target.blockId}`);
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

    void resolveHash();
    window.addEventListener("hashchange", resolveHash);
    return () => {
      generation += 1;
      window.removeEventListener("hashchange", resolveHash);
    };
  }, [pageId]);

  return null;
}
