// 结构 Diff 视图测试（M2-T05）：added/removed/changed/moved 四类渲染、
// changed 的字段级 before/after、moved 的路径 before→after、空 changes 显示「两版相同」。
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { DocumentDiff } from "../../../../contracts/generated/typescript";

import { DiffView } from "./diff-view";
import { shortId } from "./utils";

// 注意：fixture UUID 前 8 位必须互不相同（短 id 展示取前 8 位）。
const BLK_ADDED = "aaaaaaa1-c3d4-7e5f-8a9b-0c1d2e3f4a31";
const BLK_REMOVED = "bbbbbbb2-c3d4-7e5f-8a9b-0c1d2e3f4a32";
const BLK_CHANGED = "ccccccc3-c3d4-7e5f-8a9b-0c1d2e3f4a33";
const BLK_MOVED = "ddddddd4-c3d4-7e5f-8a9b-0c1d2e3f4a34";
const BLK_PARENT = "eeeeeee5-c3d4-7e5f-8a9b-0c1d2e3f4a35";

const diff: DocumentDiff = {
  changes: [
    {
      type: "added",
      blockId: BLK_ADDED,
      parentId: BLK_PARENT,
      path: [1],
    },
    {
      type: "changed",
      blockId: BLK_CHANGED,
      parentId: "",
      fields: [
        {
          field: "content",
          before: '[{"type":"text","text":"旧文本"}]',
          after: '[{"type":"text","text":"新文本"}]',
        },
      ],
    },
    {
      type: "moved",
      blockId: BLK_MOVED,
      parentId: "",
      beforePath: [0],
      afterPath: [2],
    },
    {
      type: "removed",
      blockId: BLK_REMOVED,
      parentId: "",
      path: [3],
    },
  ],
};

describe("DiffView", () => {
  it("渲染四类变更徽章与块短 id", () => {
    render(<DiffView diff={diff} />);
    expect(screen.getByText("新增")).toBeInTheDocument();
    expect(screen.getByText("删除")).toBeInTheDocument();
    expect(screen.getByText("修改")).toBeInTheDocument();
    expect(screen.getByText("移动")).toBeInTheDocument();
    expect(screen.getByText(shortId(BLK_ADDED))).toBeInTheDocument();
    expect(screen.getByText(shortId(BLK_REMOVED))).toBeInTheDocument();
    // 父块定位
    expect(screen.getByText(`父块 ${shortId(BLK_PARENT)}`)).toBeInTheDocument();
  });

  it("changed 展示字段级 before/after", () => {
    render(<DiffView diff={diff} />);
    expect(screen.getByText("content")).toBeInTheDocument();
    expect(
      screen.getByText('[{"type":"text","text":"旧文本"}]'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('[{"type":"text","text":"新文本"}]'),
    ).toBeInTheDocument();
  });

  it("moved 展示 before/after 路径", () => {
    render(<DiffView diff={diff} />);
    expect(screen.getByText("路径 [0] → [2]")).toBeInTheDocument();
  });

  it("added/removed 展示索引路径", () => {
    render(<DiffView diff={diff} />);
    expect(screen.getByText("路径 [1]")).toBeInTheDocument();
    expect(screen.getByText("路径 [3]")).toBeInTheDocument();
  });

  it("changes 为空（from==to）显示「两版相同」", () => {
    render(<DiffView diff={{ changes: [] }} />);
    expect(
      screen.getByText("两版相同，没有结构差异。"),
    ).toBeInTheDocument();
  });
});
