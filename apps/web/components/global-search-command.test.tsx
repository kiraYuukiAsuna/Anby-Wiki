import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { GlobalSearchCommand } from "./global-search-command";

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  searchPages: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push }),
}));

vi.mock("@/lib/api", () => ({
  searchApi: () => ({ searchPages: mocks.searchPages }),
}));

function renderSearch() {
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <GlobalSearchCommand />
    </SWRConfig>,
  );
}

describe("GlobalSearchCommand", () => {
  beforeEach(() => {
    mocks.push.mockReset();
    mocks.searchPages.mockReset();
    mocks.searchPages.mockResolvedValue({
      items: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
          displayTitle: "Anby Demara",
          namespace: "main",
          matchedOn: "body",
          highlight: "A quiet [[swordswoman]] from the Cunning Hares.",
          score: 2.5,
        },
      ],
      total: 1,
    });
  });

  it("通过生成客户端搜索全文并安全渲染高亮", async () => {
    renderSearch();
    fireEvent.click(screen.getByRole("button", { name: "搜索站点" }));
    fireEvent.change(screen.getByPlaceholderText("输入关键词…"), {
      target: { value: "swordswoman" },
    });

    await waitFor(() => expect(mocks.searchPages).toHaveBeenCalled(), {
      timeout: 2000,
    });
    expect(mocks.searchPages).toHaveBeenCalledWith({
      q: "swordswoman",
      namespace: "main",
      limit: 20,
    });
    expect(await screen.findByText("Anby Demara")).toBeInTheDocument();
    expect(screen.getByText("正文命中")).toBeInTheDocument();
    expect(screen.getByText("swordswoman").tagName).toBe("MARK");
    expect(screen.queryByText("[[")).not.toBeInTheDocument();
  });

  it("支持快捷键打开并在选择后跳转稳定 Page ID", async () => {
    renderSearch();
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    fireEvent.change(screen.getByPlaceholderText("输入关键词…"), {
      target: { value: "anby" },
    });
    fireEvent.click(await screen.findByText("Anby Demara"));

    expect(mocks.push).toHaveBeenCalledWith(
      "/pages/0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
    );
  });
});
