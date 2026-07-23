import { act, render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { StableAnchorResolver } from "./stable-anchor-resolver";

const resolvePageAnchor = vi.fn();

vi.mock("@/lib/api", () => ({
  projectionApi: () => ({ resolvePageAnchor }),
}));

describe("StableAnchorResolver", () => {
  beforeEach(() => {
    resolvePageAnchor.mockReset();
    window.history.replaceState(null, "", "/pages/page-1");
  });

  it("keeps an existing stable block hash without an API lookup", async () => {
    const heading = document.createElement("h2");
    heading.id = "block-1";
    heading.scrollIntoView = vi.fn();
    document.body.appendChild(heading);
    window.history.replaceState(null, "", "/pages/page-1#block-1");

    render(<StableAnchorResolver pageId="page-1" />);

    await waitFor(() => expect(heading.scrollIntoView).toHaveBeenCalled());
    expect(resolvePageAnchor).not.toHaveBeenCalled();
    heading.remove();
  });

  it("replaces a historical slug with the resolved stable block id", async () => {
    const heading = document.createElement("h2");
    heading.id = "block-2";
    heading.scrollIntoView = vi.fn();
    document.body.appendChild(heading);
    resolvePageAnchor.mockResolvedValue({
      pageId: "page-1",
      blockId: "block-2",
      currentSlug: "renamed",
      viaAlias: true,
      viaRedirect: false,
    });
    window.history.replaceState(null, "", "/wiki/Example#old-heading");

    render(<StableAnchorResolver pageId="page-1" />);

    await waitFor(() => expect(window.location.hash).toBe("#block-2"));
    expect(resolvePageAnchor).toHaveBeenCalledWith({
      id: "page-1",
      slug: "old-heading",
    });
    expect(heading.scrollIntoView).toHaveBeenCalled();
    heading.remove();
  });

  it("resolves a hash changed after mount", async () => {
    resolvePageAnchor.mockRejectedValue(new Error("not found"));
    render(<StableAnchorResolver pageId="page-1" />);

    await act(async () => {
      window.history.replaceState(null, "", "/pages/page-1#legacy");
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    await waitFor(() =>
      expect(resolvePageAnchor).toHaveBeenCalledWith({
        id: "page-1",
        slug: "legacy",
      }),
    );
  });
});
