import type {
  MergeConflict,
  ProposalOperationRecord,
} from "../../../contracts/generated/typescript";

import {
  parseDocument,
  type Block,
  type Document,
  type InlineNode,
} from "@/lib/ast/schema";

type BlockContainer = { children: Block[] };
type OperationTarget = {
  blockId?: string;
  nodeId?: string;
  entityId?: string;
  claimId?: string;
  citationId?: string;
};

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
  const target = operation.target as OperationTarget | undefined;
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
      const container = findContainer(
        document,
        payloadField(payload, "parent_block_id", "parentBlockId"),
      );
      const index = integerIndex(payload?.index, container.children.length, true);
      container.children.splice(index, 0, parseBlock(payload?.block));
      return;
    }
    case "move_block": {
      const location = requireBlock(document, target?.blockId);
      const [block] = location.container.children.splice(location.index, 1);
      const container = findContainer(
        document,
        payloadField(payload, "parent_block_id", "parentBlockId"),
      );
      const index = integerIndex(payload?.index, container.children.length, true);
      container.children.splice(index, 0, block);
      return;
    }
    case "insert_page_reference":
    case "insert_entity_reference":
    case "insert_claim_reference":
    case "insert_citation_reference": {
      const content = requireInlineContent(document, target?.blockId);
      const index = integerIndex(payload?.index, content.length, true);
      content.splice(index, 0, buildReference(type, target, payload));
      return;
    }
    case "retarget_page_reference":
    case "retarget_entity_reference":
    case "retarget_claim_reference":
    case "retarget_citation_reference":
    case "retarget_external_link": {
      const content = requireInlineContent(document, target?.blockId);
      const index = nodeIndex(target?.nodeId, content.length);
      const expectedType = referenceNodeType(type);
      if (content[index].type !== expectedType) {
        throw new Error(
          `目标行内节点类型不匹配: ${content[index].type}，需要 ${expectedType}`,
        );
      }
      content[index] = buildReference(type, target, payload);
      return;
    }
    default:
      throw new Error(`客户端工作副本合并暂不支持 Operation: ${type}`);
  }
}

function buildReference(
  operationType: string,
  target: OperationTarget | undefined,
  payload: Record<string, unknown> | undefined,
): InlineNode {
  const displayText = optionalString(
    payloadField(payload, "display_text", "displayText"),
    "display_text",
  );
  switch (operationType) {
    case "insert_page_reference":
    case "retarget_page_reference": {
      const targetPageId = optionalString(
        payloadField(payload, "target_page_id", "targetPageId"),
        "target_page_id",
      );
      if (targetPageId) {
        const targetHeadingBlockId = optionalString(
          payloadField(
            payload,
            "target_heading_block_id",
            "targetHeadingBlockId",
          ),
          "target_heading_block_id",
        );
        return {
          type: "page_reference",
          target_page_id: targetPageId,
          ...(targetHeadingBlockId
            ? { target_heading_block_id: targetHeadingBlockId }
            : {}),
          display_text: displayText ?? "",
        };
      }
      if (operationType === "retarget_page_reference") {
        throw new Error("retarget_page_reference 缺少 target_page_id");
      }
      const targetNamespace = requiredString(
        payloadField(payload, "target_namespace", "targetNamespace"),
        "target_namespace",
      );
      const normalizedTitle = requiredString(
        payloadField(payload, "normalized_title", "normalizedTitle"),
        "normalized_title",
      );
      const expectedEntityType = optionalString(
        payloadField(
          payload,
          "expected_entity_type",
          "expectedEntityType",
        ),
        "expected_entity_type",
      );
      return {
        type: "page_reference",
        resolution_status: "unresolved",
        target_namespace: targetNamespace,
        normalized_title: normalizedTitle,
        ...(expectedEntityType
          ? { expected_entity_type: expectedEntityType }
          : {}),
      };
    }
    case "insert_entity_reference":
    case "retarget_entity_reference":
      return {
        type: "entity_reference",
        entity_id: requiredString(target?.entityId, "entity_id"),
        display_text: displayText ?? "",
      };
    case "insert_claim_reference":
    case "retarget_claim_reference":
      return {
        type: "claim_reference",
        claim_id: requiredString(target?.claimId, "claim_id"),
        display_text: displayText ?? "",
      };
    case "insert_citation_reference":
    case "retarget_citation_reference":
      return {
        type: "citation_reference",
        citation_id: requiredString(target?.citationId, "citation_id"),
        ...(displayText === undefined ? {} : { display_text: displayText }),
      };
    case "retarget_external_link":
      return {
        type: "external_link",
        url: requiredString(payload?.url, "url"),
        display_text: displayText ?? "",
      };
    default:
      throw new Error(`未知引用 Operation: ${operationType}`);
  }
}

function referenceNodeType(operationType: string): InlineNode["type"] {
  switch (operationType) {
    case "retarget_page_reference":
      return "page_reference";
    case "retarget_entity_reference":
      return "entity_reference";
    case "retarget_claim_reference":
      return "claim_reference";
    case "retarget_citation_reference":
      return "citation_reference";
    case "retarget_external_link":
      return "external_link";
    default:
      throw new Error(`Operation 不是重定向引用操作: ${operationType}`);
  }
}

function requireInlineContent(document: Document, blockId: unknown): InlineNode[] {
  const location = requireBlock(document, blockId);
  const block = location.container.children[location.index];
  if (block.type !== "heading" && block.type !== "paragraph") {
    throw new Error(`目标 Block 不支持行内引用: ${block.id}`);
  }
  return block.content;
}

function nodeIndex(value: unknown, length: number): number {
  if (typeof value !== "string" || !/^(0|[1-9]\d*)$/.test(value)) {
    throw new Error("Operation node_id 非法");
  }
  return integerIndex(Number(value), length, false);
}

function payloadField(
  payload: Record<string, unknown> | undefined,
  wireName: string,
  clientName: string,
): unknown {
  return payload?.[wireName] ?? payload?.[clientName];
}

function requiredString(value: unknown, name: string): string {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`Operation 缺少 ${name}`);
  }
  return value;
}

function optionalString(value: unknown, name: string): string | undefined {
  if (value === undefined || value === null) return undefined;
  if (typeof value !== "string") throw new Error(`Operation ${name} 非法`);
  return value;
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
