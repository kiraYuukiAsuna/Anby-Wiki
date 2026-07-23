import type {
  MergeConflict,
  ProposalOperationRecord,
} from "../../../contracts/generated/typescript";

import { parseDocument, type Block, type Document } from "@/lib/ast/schema";

type BlockContainer = { children: Block[] };

export function applyPageProposalOperations(
  document: Document,
  records: ProposalOperationRecord[],
  conflicts: MergeConflict[] = [],
): Document {
  const result = structuredClone(document);
  const choices = conflictChoices(conflicts);
  for (const record of [...records].sort((left, right) => left.sequence - right.sequence)) {
    const blockId = (
      record.operation as unknown as { target?: { blockId?: string } }
    ).target?.blockId;
    const choice = blockId ? choices.get(blockId) : undefined;
    if (choice === "choose_current" || choice === "dismiss") continue;
    applyOperation(result, record.operation as unknown as Record<string, unknown>);
  }
  return parseDocument(result);
}

function conflictChoices(conflicts: MergeConflict[]): Map<string, string> {
  const choices = new Map<string, string>();
  for (const conflict of conflicts) {
    if (!conflict.targetBlockId || conflict.status === "open") continue;
    const resolution = conflict.resolution as { choice?: string } | undefined;
    if (resolution?.choice) choices.set(conflict.targetBlockId, resolution.choice);
  }
  return choices;
}

function applyOperation(document: Document, operation: Record<string, unknown>): void {
  const type = String(operation.operationType ?? "");
  const target = operation.target as { blockId?: string } | undefined;
  const payload = operation.payload as Record<string, unknown> | undefined;
  switch (type) {
    case "replace_block": {
      const location = requireBlock(document, target?.blockId);
      location.container.children[location.index] = parseBlock(payload?.block);
      return;
    }
    case "delete_block": {
      const location = requireBlock(document, target?.blockId);
      location.container.children.splice(location.index, 1);
      return;
    }
    case "insert_block": {
      const container = findContainer(document, payload?.parentBlockId);
      const index = integerIndex(payload?.index, container.children.length, true);
      container.children.splice(index, 0, parseBlock(payload?.block));
      return;
    }
    case "move_block": {
      const location = requireBlock(document, target?.blockId);
      const [block] = location.container.children.splice(location.index, 1);
      const container = findContainer(document, payload?.parentBlockId);
      const index = integerIndex(payload?.index, container.children.length, true);
      container.children.splice(index, 0, block);
      return;
    }
    default:
      throw new Error(`客户端工作副本合并暂不支持 Operation: ${type}`);
  }
}

function requireBlock(
  document: Document,
  id: unknown,
): { container: BlockContainer; index: number } {
  if (typeof id !== "string") throw new Error("Operation 缺少 block_id");
  const found = findBlock(document, id);
  if (!found) throw new Error(`目标 Block 不存在: ${id}`);
  return found;
}

function findBlock(
  container: BlockContainer,
  id: string,
): { container: BlockContainer; index: number } | null {
  for (let index = 0; index < container.children.length; index += 1) {
    const block = container.children[index];
    if (block.id === id) return { container, index };
    if ("children" in block && Array.isArray(block.children)) {
      const nested = findBlock(block as BlockContainer, id);
      if (nested) return nested;
    }
  }
  return null;
}

function findContainer(document: Document, parentId: unknown): BlockContainer {
  if (parentId == null || parentId === "") return document;
  if (typeof parentId !== "string") throw new Error("parent_block_id 非法");
  const found = findBlock(document, parentId);
  if (!found) throw new Error(`父 Block 不存在: ${parentId}`);
  const block = found.container.children[found.index];
  if (!("children" in block) || !Array.isArray(block.children)) {
    throw new Error(`父 Block 不能包含子块: ${parentId}`);
  }
  return block as BlockContainer;
}

function parseBlock(value: unknown): Block {
  const document = parseDocument({
    type: "document",
    schema_version: 1,
    children: [value],
  });
  return document.children[0];
}

function integerIndex(value: unknown, length: number, allowEnd: boolean): number {
  if (!Number.isInteger(value)) throw new Error("Operation index 非法");
  const index = Number(value);
  const maximum = allowEnd ? length : length - 1;
  if (index < 0 || index > maximum) throw new Error("Operation index 越界");
  return index;
}
