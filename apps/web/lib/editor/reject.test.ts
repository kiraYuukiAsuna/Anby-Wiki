// 显式拒绝与映射表完整性测试（ADR-0005 验收标准 1、3）：
// 不支持的 BlockNote 特性在 Adapter 边界抛错，不静默丢失；
// 每种 AST block 都有映射，每个 BlockNote 默认块要么进 schema 要么在拒绝清单。
import { defaultBlockSpecs } from "@blocknote/core";
import { describe, expect, it } from "vitest";

import { UnsupportedBlockNoteFeatureError } from "./errors";
import { newBlockId } from "./ids";
import {
  AST_BLOCK_TYPES,
  AST_TO_BLOCKNOTE_BLOCK,
  editorSchema,
  UNMAPPED_BLOCKNOTE_BLOCKS,
  type BNBlock,
} from "./schema";
import { toAst } from "./toAst";

function bnBlock(partial: Partial<BNBlock> & { type: string }): BNBlock {
  return {
    id: newBlockId(),
    props: {},
    content: [],
    children: [],
    ...partial,
  };
}

describe("映射表完整性", () => {
  it("每种 AST block 类型都有映射", () => {
    for (const type of AST_BLOCK_TYPES) {
      expect(AST_TO_BLOCKNOTE_BLOCK[type]).toBeTruthy();
    }
    expect(Object.keys(AST_TO_BLOCKNOTE_BLOCK).sort()).toEqual(
      [...AST_BLOCK_TYPES].sort(),
    );
  });

  it("BlockNote 默认块类型要么进入 editorSchema，要么在显式拒绝清单", () => {
    for (const key of Object.keys(defaultBlockSpecs)) {
      const inSchema = key in editorSchema.blockSpecs;
      const rejected = (UNMAPPED_BLOCKNOTE_BLOCKS as readonly string[]).includes(
        key,
      );
      // 原生 table 被自定义容器 table 替代：同名覆盖视为已处理。
      const replaced = key === "table" && inSchema;
      expect(inSchema || rejected || replaced).toBe(true);
    }
  });
});

describe("不映射特性显式拒绝", () => {
  it("未映射的 block 类型（checkListItem）报错而非丢弃", () => {
    expect(() =>
      toAst([bnBlock({ type: "checkListItem", content: [] })]),
    ).toThrow(UnsupportedBlockNoteFeatureError);
    expect(() => toAst([bnBlock({ type: "checkListItem" })])).toThrow(
      /checkListItem/,
    );
  });

  it("其他未映射块类型（audio / image / pageBreak）同样报错", () => {
    for (const type of ["audio", "image", "pageBreak", "toggleListItem"]) {
      expect(() => toAst([bnBlock({ type })])).toThrow(
        UnsupportedBlockNoteFeatureError,
      );
    }
  });

  it("不支持的文本样式（underline）报错", () => {
    const block = bnBlock({
      type: "paragraph",
      content: [{ type: "text", text: "x", styles: { underline: true } }],
    });
    expect(() => toAst([block])).toThrow(/underline/);
  });

  it("inlineCode 与其他样式叠加报错", () => {
    const block = bnBlock({
      type: "paragraph",
      content: [
        { type: "text", text: "x", styles: { inlineCode: true, bold: true } },
      ],
    });
    expect(() => toAst([block])).toThrow(UnsupportedBlockNoteFeatureError);
  });

  it("link 内带样式文本报错（不静默丢样式）", () => {
    const block = bnBlock({
      type: "paragraph",
      content: [
        {
          type: "link",
          href: "https://example.com",
          content: [{ type: "text", text: "x", styles: { bold: true } }],
        },
      ],
    });
    expect(() => toAst([block])).toThrow(/external_link/);
  });

  it("未知行内内容类型报错", () => {
    const block = bnBlock({
      type: "paragraph",
      content: [{ type: "mention", props: { id: "u1" } }],
    });
    expect(() => toAst([block])).toThrow(/mention/);
  });

  it("非默认视觉 prop（textColor）报错", () => {
    const block = bnBlock({
      type: "paragraph",
      props: { textColor: "red" },
      content: [],
    });
    expect(() => toAst([block])).toThrow(/textColor/);
  });

  it("numberedListItem 的 start prop 报错", () => {
    const block = bnBlock({
      type: "numberedListItem",
      props: { start: 3 },
      content: [],
    });
    expect(() => toAst([block])).toThrow(/start/);
  });

  it("heading isToggleable=true 报错", () => {
    const block = bnBlock({
      type: "heading",
      props: { level: 2, isToggleable: true },
      content: [],
    });
    expect(() => toAst([block])).toThrow(/isToggleable/);
  });

  it("callout kind 越界报错", () => {
    const block = bnBlock({
      type: "callout",
      props: { kind: "success" },
      children: [],
    });
    expect(() => toAst([block])).toThrow(/success/);
  });
});
