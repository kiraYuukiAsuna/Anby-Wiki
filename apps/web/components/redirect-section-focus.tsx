"use client";

import { useEffect } from "react";

export function RedirectSectionFocus({ blockId }: { blockId?: string }) {
  useEffect(() => {
    if (!blockId) return;
    const frame = window.requestAnimationFrame(() => {
      const element = document.getElementById(blockId);
      if (!element) return;
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}#${blockId}`,
      );
      element.scrollIntoView({ block: "start" });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [blockId]);

  return null;
}
