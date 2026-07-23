import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  Collection,
  CollectionMembership,
  CollectionRule,
} from "../../../../contracts/generated/typescript";

import { CollectionList } from "./collection-list";
import { CollectionMembers } from "./collection-members";

const mocks = vi.hoisted(() => ({
  listCollections: vi.fn(),
  listCollectionMembers: vi.fn(),
  toast: { error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  collectionsApi: () => ({
    listCollections: mocks.listCollections,
    listCollectionMembers: mocks.listCollectionMembers,
  }),
}));
vi.mock("sonner", () => ({ toast: mocks.toast }));

const COLLECTION_ID = "11111111-1111-7111-8111-111111111111";
const PAGE_ID = "22222222-2222-7222-8222-222222222222";
const ENTITY_ID = "33333333-3333-7333-8333-333333333333";
const REVISION_ID = "44444444-4444-7444-8444-444444444444";

function makeCollection(
  id: string,
  title: string,
  query: CollectionRule | null = null,
): Collection {
  return {
    id,
    wikiId: "55555555-5555-7555-8555-555555555555",
    collectionType: query ? "rule" : "manual",
    title,
    descriptionPageId: null,
    query,
    createdAt: new Date("2026-07-24T00:00:00Z"),
    updatedAt: new Date("2026-07-24T00:00:00Z"),
  } as unknown as Collection;
}

function makeMember(
  memberType: "page" | "entity",
  id: string,
): CollectionMembership {
  return {
    memberType,
    pageId: memberType === "page" ? id : null,
    entityId: memberType === "entity" ? id : null,
    displayTitle: memberType === "page" ? "页面成员" : "实体成员",
    sourceType: memberType === "page" ? "manual" : "rule",
    sortKey: memberType === "page" ? "01" : "02",
    sourceRevisionId: REVISION_ID,
    createdAt: new Date("2026-07-24T00:00:00Z"),
  } as unknown as CollectionMembership;
}

describe("CollectionList", () => {
  beforeEach(() => vi.clearAllMocks());

  it("展示 Manual/Rule 摘要并按游标追加下一页", async () => {
    const manual = makeCollection(COLLECTION_ID, "精选");
    const rule = makeCollection(
      "66666666-6666-7666-8666-666666666666",
      "角色",
      { version: 1, kind: "entity_type", entityType: "character" },
    );
    mocks.listCollections.mockResolvedValue({
      items: [rule],
      nextCursor: null,
    });
    render(
      <CollectionList
        initialPage={{ items: [manual], nextCursor: "next-page" }}
      />,
    );

    expect(screen.getByText("人工维护")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));

    expect(await screen.findByText("实体类型：character")).toBeInTheDocument();
    expect(mocks.listCollections).toHaveBeenCalledWith({
      cursor: "next-page",
      pageSize: 20,
    });
    expect(
      screen.queryByRole("button", { name: "加载更多" }),
    ).not.toBeInTheDocument();
  });

  it("空列表显示空态", () => {
    render(<CollectionList initialPage={{ items: [], nextCursor: "" }} />);
    expect(screen.getByText("当前还没有 Collection。")).toBeInTheDocument();
  });
});

describe("CollectionMembers", () => {
  beforeEach(() => vi.clearAllMocks());

  it("展示 Page/Entity 链接、排序键与来源 Revision", () => {
    render(
      <CollectionMembers
        collectionId={COLLECTION_ID}
        initialPage={{
          items: [
            makeMember("page", PAGE_ID),
            makeMember("entity", ENTITY_ID),
          ],
          nextCursor: "",
        }}
      />,
    );

    expect(screen.getByRole("link", { name: "页面成员" })).toHaveAttribute(
      "href",
      `/pages/${PAGE_ID}`,
    );
    expect(screen.getByRole("link", { name: "实体成员" })).toHaveAttribute(
      "href",
      `/entities/${ENTITY_ID}`,
    );
    expect(screen.getAllByText(`Revision ${REVISION_ID.slice(0, 8)}`)).toHaveLength(2);
    expect(screen.getByText(/排序键 01/)).toBeInTheDocument();
  });

  it("成员续页失败时保留现有列表并提示", async () => {
    mocks.listCollectionMembers.mockRejectedValue(new Error("network"));
    render(
      <CollectionMembers
        collectionId={COLLECTION_ID}
        initialPage={{
          items: [makeMember("page", PAGE_ID)],
          nextCursor: "next-members",
        }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "加载更多成员" }));

    await vi.waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith("加载成员失败", {
        description: "请稍后重试。",
      }),
    );
    expect(screen.getByText("页面成员")).toBeInTheDocument();
  });
});
