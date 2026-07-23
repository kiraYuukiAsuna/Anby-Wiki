import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type {
  Proposal,
  ProposalPreview as ProposalPreviewModel,
} from "../../../../contracts/generated/typescript";

import { ProposalPreview } from "./proposal-preview";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));

const proposal: Proposal = {
  id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
  targetType: "page",
  targetId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
  baseRevisionId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a03",
  status: "in_review",
  riskLevel: "high",
  riskReasons: ["verified_claim"],
  policyDecision: {},
  createdBy: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a04",
  idempotencyKey: "test-proposal",
  createdAt: new Date("2026-07-22T00:00:00Z"),
  updatedAt: new Date("2026-07-22T00:00:00Z"),
  operations: [],
  conflicts: [],
};

const ast = { schema_version: 1, type: "document", blocks: [] };
const preview: ProposalPreviewModel = {
  proposalId: proposal.id,
  targetType: "page",
  riskLevel: "high",
  stale: true,
  base: { revisionId: proposal.baseRevisionId!, contentHash: "a".repeat(64), ast },
  current: {
    revisionId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a05",
    contentHash: "b".repeat(64),
    ast,
  },
  proposed: { revisionId: "", contentHash: "c".repeat(64), ast },
  baseToCurrent: { changes: [] },
  baseToProposed: {
    changes: [
      {
        type: "added",
        blockId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a06",
        parentId: "",
        path: [0],
      },
    ],
  },
  evidence: [{ citationId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a07", note: "权威来源" }],
  impact: {
    operationCount: 1,
    addedBlocks: 1,
    removedBlocks: 0,
    changedBlocks: 0,
    movedBlocks: 0,
  },
};

describe("ProposalPreview", () => {
  it("同时展示三态、过期提示、双 Diff、影响统计与证据", () => {
    render(<ProposalPreview proposal={proposal} preview={preview} />);

    expect(screen.getByText("Base（提案基线）")).toBeInTheDocument();
    expect(screen.getByText("Current（当前线上）")).toBeInTheDocument();
    expect(screen.getByText("Proposed（应用后）")).toBeInTheDocument();
    expect(screen.getByText("基线已过期")).toBeInTheDocument();
    expect(screen.getByText("Base → Current")).toBeInTheDocument();
    expect(screen.getByText("Base → Proposed")).toBeInTheDocument();
    expect(screen.getByText("新增", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("证据链（1）")).toBeInTheDocument();
    expect(screen.getByText("备注: 权威来源")).toBeInTheDocument();
  });
});
