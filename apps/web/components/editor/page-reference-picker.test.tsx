// 页面引用选择器测试（M2-T04，mock api 层）：
// 搜索渲染（防抖后调 pages/search）、选中既有页面进入显示文本步骤、
// 插入已解析引用（display_text 可改、与目标 ID 分离）、创建未解析引用。
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PageSearchHit } from "../../../../contracts/generated/typescript";

import { PageReferencePicker } from "./page-reference-picker";

const searchPagesMock = vi.fn();

vi.mock("@/lib/api", () => ({
  searchApi: () => ({ searchPages: searchPagesMock }),
}));

const HITS: PageSearchHit[] = [
  {
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
    displayTitle: "Anby Demara",
    namespace: "main",
    matchedOn: "title",
    highlight: "[[Anby]] Demara",
    score: 5,
  },
  {
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02",
    displayTitle: "Anby Story",
    namespace: "main",
    matchedOn: "alias",
    highlight: "[[Anby]] Story",
    score: 4,
  },
] as PageSearchHit[];

function renderPicker(overrides: Partial<Parameters<typeof PageReferencePicker>[0]> = {}) {
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    onInsertResolved: vi.fn(),
    onInsertUnresolved: vi.fn(),
    ...overrides,
  };
  // 每个用例独立 SWR 缓存：避免相同 key 在 2s dedupe 窗口内被去重而不发请求。
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <PageReferencePicker {...props} />
    </SWRConfig>,
  );
  return props;
}

async function typeQuery(text: string) {
  const input = screen.getByPlaceholderText("输入页面标题搜索…");
  fireEvent.change(input, { target: { value: text } });
  // 防抖 200ms：等待搜索请求发出。
  await waitFor(() => expect(searchPagesMock).toHaveBeenCalled(), {
    timeout: 2000,
  });
}

describe("PageReferencePicker", () => {
  beforeEach(() => {
    searchPagesMock.mockReset();
    searchPagesMock.mockResolvedValue({ items: HITS });
  });

  it("输入即搜索（防抖后经 api 层查询），渲染结果选项", async () => {
    renderPicker();
    await typeQuery("anby");
    expect(searchPagesMock).toHaveBeenCalledWith({
      q: "anby",
      namespace: "main",
      fields: ["title", "alias"],
      limit: 10,
    });
    expect(await screen.findByText("Anby Demara")).toBeInTheDocument();
    expect(screen.getByText("Anby Story")).toBeInTheDocument();
    // 列表底部常驻「创建未解析引用」选项。
    expect(screen.getByText("创建未解析引用：anby")).toBeInTheDocument();
  });

  it("选中既有页面 → 显示文本步骤 → 插入已解析引用（display_text 可改）", async () => {
    const props = renderPicker();
    await typeQuery("anby");
    fireEvent.click(await screen.findByText("Anby Demara"));

    // 第二步：默认显示文本为页面标题，可修改（与 target_page_id 分离）。
    const displayInput = screen.getByLabelText("显示文本");
    expect(displayInput).toHaveValue("Anby Demara");
    fireEvent.change(displayInput, { target: { value: "自定义文本" } });
    fireEvent.click(screen.getByRole("button", { name: "插入引用" }));

    expect(props.onInsertResolved).toHaveBeenCalledWith(
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
      "自定义文本",
    );
    expect(props.onOpenChange).toHaveBeenCalledWith(false);
  });

  it("显示文本留空时回退为页面标题", async () => {
    const props = renderPicker();
    await typeQuery("anby");
    fireEvent.click(await screen.findByText("Anby Demara"));
    const displayInput = screen.getByLabelText("显示文本");
    fireEvent.change(displayInput, { target: { value: "   " } });
    fireEvent.click(screen.getByRole("button", { name: "插入引用" }));
    expect(props.onInsertResolved).toHaveBeenCalledWith(
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
      "Anby Demara",
    );
  });

  it("选择「创建未解析引用」回调原始输入文本", async () => {
    const props = renderPicker();
    await typeQuery("  Ghost Page  ");
    fireEvent.click(screen.getByText("创建未解析引用：Ghost Page"));
    expect(props.onInsertUnresolved).toHaveBeenCalledWith("Ghost Page");
    expect(props.onOpenChange).toHaveBeenCalledWith(false);
  });

  it("无命中时提示并仍提供创建未解析引用", async () => {
    searchPagesMock.mockResolvedValue({ items: [] });
    renderPicker();
    await typeQuery("nothing matches");
    expect(await screen.findByText("没有匹配的既有页面")).toBeInTheDocument();
    expect(
      screen.getByText("创建未解析引用：nothing matches"),
    ).toBeInTheDocument();
  });
});
