import { describe, expect, it } from "vitest";
import type { ProposalOperationRecord } from "../../../contracts/generated/typescript";

import { parseDocument } from "@/lib/ast/schema";

import { applyPageProposalOperations } from "./governance-page-patch";

const firstId = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d61";
const secondId = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d62";
const thirdId = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d63";

describe("applyPageProposalOperations", () => {
  it("applies replace, insert, move and delete without mutating current", () => {
    const current = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        paragraph(firstId, "human current"),
        paragraph(secondId, "remove"),
      ],
    });
    const records = [
      operation(1, "replace_block", firstId, {
        block: paragraph(firstId, "AI merged"),
      }),
      operation(2, "insert_block", undefined, {
        index: 2,
        block: paragraph(thirdId, "new"),
      }),
      operation(3, "move_block", thirdId, { index: 0 }),
      operation(4, "delete_block", secondId, {}),
    ];

    const merged = applyPageProposalOperations(current, records);

    expect(merged.children.map((block) => block.id)).toEqual([thirdId, firstId]);
    expect(JSON.stringify(merged)).toContain("AI merged");
    expect(JSON.stringify(current)).toContain("human current");
    expect(current.children).toHaveLength(2);
  });

  it("rejects unsupported operations explicitly", () => {
    const current = parseDocument({
      type: "document",
      schema_version: 1,
      children: [paragraph(firstId, "text")],
    });
    expect(() =>
      applyPageProposalOperations(current, [
        operation(1, "retarget_page_reference", firstId, {}),
      ]),
    ).toThrow(/暂不支持/);
  });
});

function paragraph(id: string, text: string) {
  return { id, type: "paragraph", content: [{ type: "text", text }] };
}

function operation(
  sequence: number,
  operationType: string,
  blockId: string | undefined,
  payload: Record<string, unknown>,
): ProposalOperationRecord {
  return {
    id: crypto.randomUUID(),
    sequence,
    operation: {
      schemaVersion: 1,
      operationType,
      base: {},
      target: blockId ? { blockId } : {},
      expectedHash: "hash",
      evidence: [],
      risk: { level: "low", reasons: [] },
      payload,
    } as never,
  };
}
